//go:build e2e

// Package e2e содержит сквозные тесты OrderService с реальной Kafka (Redpanda).
//
// Цель отдельной сьюты — проверить асинхронную цепочку:
//
//	HTTP Pay → orderProducer (Kafka topic order.paid)
//	         → реальный AssemblyService
//	         → Kafka topic assembly.ship-assembled
//	         → order/internal/consumer/assembly_consumer
//	         → CommitParts + UPDATE orders SET status=ASSEMBLED
//
// На неделе 6 в цепочке появилась session-авторизация: HTTP защищён Bearer-middleware,
// Inventory gRPC — auth-interceptor, а session_uuid пробрасывается через Kafka headers
// (см. platform/pkg/middleware/kafka). Поэтому в setup поднят ещё IAM (Postgres + Redis
// + bufconn-сервер), а bufconn-клиент Inventory снабжён SessionForwarder
//
// Запускается только под тегом сборки e2e (см. solutions/week_6/Taskfile.yaml).
// В обычном go test ./... не собирается, чтобы не платить временем старта Redpanda
package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	assemblyApp "github.com/student/assembly/pkg/app"
	iamApp "github.com/student/iam/pkg/app"
	invApp "github.com/student/inventory/pkg/app"
	inventoryClientPkg "github.com/student/order/internal/client/grpc/inventory/v1"
	assemblyconsumer "github.com/student/order/internal/consumer/assembly_consumer"
	"github.com/student/order/internal/interceptor"
	orderProducer "github.com/student/order/internal/producer/order_producer"
	orderRepoPkg "github.com/student/order/internal/repository/order"
	"github.com/student/order/pkg/app"
	payApp "github.com/student/payment/pkg/app"
	wrappedKafkaConsumer "github.com/student/platform/pkg/kafka/consumer"
	wrappedKafkaProducer "github.com/student/platform/pkg/kafka/producer"
	kafkaMiddleware "github.com/student/platform/pkg/middleware/kafka"
	authv1 "github.com/student/shared/pkg/proto/auth/v1"
	inventoryv1 "github.com/student/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/student/shared/pkg/proto/payment/v1"
	userv1 "github.com/student/shared/pkg/proto/user/v1"
)

// Предзагруженные UUID и цены деталей (из migrations/inventory/00002_seed_parts.sql).
// Дублируются с tests/api_test.go намеренно: e2e — самостоятельная сьюта,
// которая не должна зависеть от соседнего пакета.
const (
	HullAluminumUUID = "550e8400-e29b-41d4-a716-446655440001" // 500000 kopecks
	EngineIonCUUID   = "550e8400-e29b-41d4-a716-446655440003" // 300000 kopecks

	HullAluminumPrice = 500000
	EngineIonCPrice   = 300000
)

const (
	bufSize = 1024 * 1024

	// Redpanda на macOS поднимается ~5-10 секунд, оставим запас
	redpandaImage = "docker.redpanda.com/redpandadata/redpanda:v25.1.7"

	// Redis для IAM-сессий — версия совпадает с solutions/week_6/iam.env
	redisImage = "redis:8.6.3-alpine3.23"

	// numPartitions=1 достаточно: тесту важен факт доставки, не масштабирование
	topicPartitions        = 1
	topicReplicationFactor = 1

	// sessionTTL — для e2e достаточно часа: тест короткий, сессия не успеет
	// протухнуть, повторных логинов между шагами не делаем
	sessionTTL = time.Hour
)

var (
	httpClient = &http.Client{Timeout: 10 * time.Second}
	ts         *httptest.Server

	// Уникальные топики и group-id на прогон — изолируют параллельные CI-сборки,
	// которые могут шарить один и тот же Redpanda-кластер
	orderPaidTopic     string
	shipAssembledTopic string
	assemblyGroupID    string
	orderGroupID       string

	// Прямой пул к БД order — нужен в редких случаях для проверки состояния,
	// которое не выставляется через API (например, аудит конкретных полей)
	orderDBPool     *pgxpool.Pool
	inventoryDBPool *pgxpool.Pool

	inventoryClient inventoryv1.InventoryServiceClient

	// IAM-клиенты экспортируем для lifecycle_test — там делаем Register/Login,
	// чтобы получить sessionUUID для Bearer-заголовка
	authSvcClient authv1.AuthServiceClient
	userSvcClient userv1.UserServiceClient
)

// runMain — обёртка над m.Run, чтобы defer-cleanup отработал даже при panic в setup.
// os.Exit обходит defer, поэтому Exit зовём из TestMain отдельно
func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanups := newCleanupStack()
	defer cleanups.run(context.Background())

	// 1-3. PostgreSQL × 3 (order + inventory + iam) — три отдельных контейнера
	orderPool := startPostgresAndMigrate(ctx, cleanups, "order-service", "../../../migrations/order")
	inventoryPool := startPostgresAndMigrate(ctx, cleanups, "inventory-service", "../../../migrations/inventory")
	iamPool := startPostgresAndMigrate(ctx, cleanups, "iam-service", "../../../migrations/iam")
	orderDBPool = orderPool
	inventoryDBPool = inventoryPool

	txManager := mustNew(manager.New(trmpgx.NewDefaultFactory(orderPool)))

	// 4. Redis — стораж сессий IAM
	redisClient := startRedis(ctx, cleanups)

	// 5. IAM gRPC через bufconn — нужен и Inventory-серверу (auth-interceptor),
	// и Order HTTP-обработчику (middleware), и тесту (Register/Login).
	// bcrypt.MinCost критичен — иначе тест становится в разы медленнее
	iamConn := startBufconnGRPCIAM(ctx, cleanups, iamPool, redisClient)
	authSvcClient = authv1.NewAuthServiceClient(iamConn)
	userSvcClient = userv1.NewUserServiceClient(iamConn)

	// 6. Inventory + Payment gRPC через bufconn — Kafka их не касается,
	// поэтому остаются in-memory (быстрее, чем поднимать ещё контейнеры).
	// Inventory снабжён auth-interceptor'ом на сервере и SessionForwarder'ом
	// на клиенте — иначе CommitParts из assembly_consumer и GetPart из теста
	// не прошли бы аутентификацию
	invConn := startBufconnGRPCInventory(ctx, cleanups, inventoryPool, authSvcClient)
	payConn := startBufconnGRPCPayment(ctx, cleanups)

	inventoryClient = inventoryv1.NewInventoryServiceClient(invConn)
	paymentClient := paymentv1.NewPaymentServiceClient(payConn)

	// 7. Redpanda — реальная Kafka-совместимая инфраструктура
	broker := startRedpanda(ctx, cleanups)

	// 8. Уникальные топики на прогон + явное создание через AdminClient.
	// Sarama-консьюмер падает, если топика ещё нет, поэтому полагаться на
	// auto-create нельзя — нужно дождаться готовности
	suffix := time.Now().UnixNano()
	orderPaidTopic = fmt.Sprintf("e2e-%d-order.paid", suffix)
	shipAssembledTopic = fmt.Sprintf("e2e-%d-assembly.ship-assembled", suffix)
	assemblyGroupID = fmt.Sprintf("e2e-%d-assembly-service", suffix)
	orderGroupID = fmt.Sprintf("e2e-%d-order-service", suffix)
	createTopics(broker, orderPaidTopic, shipAssembledTopic)

	// 9. Реальный Sarama-продьюсер для order — отправляет OrderPaid в Kafka
	syncProducer := mustNew(sarama.NewSyncProducer([]string{broker}, producerConfig()))
	cleanups.add("order sarama producer", func(_ context.Context) error { return syncProducer.Close() })

	orderPaidKafkaProducer := wrappedKafkaProducer.NewProducer(syncProducer, orderPaidTopic)
	realOrderProducer := orderProducer.New(orderPaidKafkaProducer)

	// 10. Order HTTP-сервер с реальным продьюсером (НЕ noopProducer как в api_test).
	// Дополнительно подключаем authSvcClient — на неделе 6 HTTP-обработчик обёрнут
	// в Bearer-middleware
	handler := mustNew(app.NewHTTPHandlerWithProducer(orderPool, txManager, inventoryClient, paymentClient, authSvcClient, realOrderProducer))
	ts = httptest.NewServer(handler)
	cleanups.add("httptest server", func(_ context.Context) error { ts.Close(); return nil })

	// 11. Order ShipAssembled-консьюмер — реальный код из internal/consumer/assembly_consumer.
	// Слушает топик ShipAssembled и переводит заказ в ASSEMBLED через CommitParts
	startOrderShipAssembledConsumer(ctx, cleanups, broker, orderPool, txManager, inventoryClient)

	// 12. Реальный AssemblyService — тот же код, что в проде.
	// Используется тот же код, что и в проде: consumer/order_paid → service/assembly →
	// producer/ship_assembled. Контракт обоих proto-сообщений проверяется через
	// реальные decode.go / encode-логику assembly-сервиса
	startAssemblyService(ctx, cleanups, broker)

	return m.Run()
}

// =============================================================================
// Containers & infrastructure helpers
// =============================================================================

func startPostgres(ctx context.Context, dbName, user, password string) (*tcpostgres.PostgresContainer, string, error) {
	c, err := tcpostgres.Run(
		ctx,
		"postgres:18.3-alpine3.23",
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, "", err
	}

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, "", err
	}

	return c, dsn, nil
}

func startPostgresAndMigrate(ctx context.Context, cleanups *cleanupStack, name, migrationsDir string) *pgxpool.Pool {
	container, dsn, err := startPostgres(ctx, name, name+"-user", name+"-password")
	if err != nil {
		panic(fmt.Errorf("postgres %s: %w", name, err))
	}
	cleanups.add("postgres "+name, func(c context.Context) error { return container.Terminate(c) })

	if err = runMigrations(dsn, migrationsDir); err != nil {
		panic(fmt.Errorf("migrate %s: %w", name, err))
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(fmt.Errorf("pgxpool %s: %w", name, err))
	}
	cleanups.add("pgxpool "+name, func(_ context.Context) error { pool.Close(); return nil })

	return pool
}

func runMigrations(dsn, migrationsDir string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return err
	}

	return goose.Up(db, absDir)
}

// startRedis поднимает Redis-контейнер для сессий IAM. Возвращает уже подключённый
// redis-клиент: и контейнер, и клиент закрываются через cleanups
func startRedis(ctx context.Context, cleanups *cleanupStack) *redis.Client {
	container, err := tcredis.Run(ctx, redisImage)
	if err != nil {
		panic(fmt.Errorf("redis: %w", err))
	}
	cleanups.add("redis container", func(c context.Context) error { return container.Terminate(c) })

	addr, err := container.ConnectionString(ctx)
	if err != nil {
		panic(fmt.Errorf("redis connection string: %w", err))
	}

	const prefix = "redis://"
	if len(addr) > len(prefix) {
		addr = addr[len(prefix):]
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	cleanups.add("redis client", func(_ context.Context) error { return client.Close() })

	return client
}

func startRedpanda(ctx context.Context, cleanups *cleanupStack) string {
	container, err := tcredpanda.Run(
		ctx, redpandaImage,
		tcredpanda.WithAutoCreateTopics(),
	)
	if err != nil {
		panic(fmt.Errorf("redpanda: %w", err))
	}
	cleanups.add("redpanda", func(c context.Context) error { return container.Terminate(c) })

	broker, err := container.KafkaSeedBroker(ctx)
	if err != nil {
		panic(fmt.Errorf("redpanda broker addr: %w", err))
	}

	return broker
}

func createTopics(broker string, topics ...string) {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0

	admin, err := sarama.NewClusterAdmin([]string{broker}, cfg)
	if err != nil {
		panic(fmt.Errorf("cluster admin: %w", err))
	}
	defer func() { _ = admin.Close() }()

	for _, topic := range topics {
		err = admin.CreateTopic(topic, &sarama.TopicDetail{
			NumPartitions:     topicPartitions,
			ReplicationFactor: topicReplicationFactor,
		}, false)
		if err != nil {
			panic(fmt.Errorf("create topic %q: %w", topic, err))
		}
	}
}

// startBufconnGRPCIAM поднимает IAM gRPC-сервер через bufconn и возвращает
// клиентское соединение. bcrypt.MinCost — критичен для скорости тестов
func startBufconnGRPCIAM(ctx context.Context, cleanups *cleanupStack, pool *pgxpool.Pool, redisClient *redis.Client) *grpc.ClientConn {
	lis := bufconn.Listen(bufSize)
	server := iamApp.NewGRPCServer(pool, redisClient, sessionTTL, bcrypt.MinCost)

	go func() {
		if err := server.Serve(lis); err != nil {
			panic(fmt.Errorf("iam grpc serve: %w", err))
		}
	}()
	cleanups.add("iam grpc server", func(_ context.Context) error { server.Stop(); return nil })

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(fmt.Errorf("iam grpc client: %w", err))
	}
	cleanups.add("iam grpc conn", func(_ context.Context) error { return conn.Close() })

	_ = ctx
	return conn
}

func startBufconnGRPCInventory(ctx context.Context, cleanups *cleanupStack, pool *pgxpool.Pool, authClient authv1.AuthServiceClient) *grpc.ClientConn {
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer(invApp.Interceptors(authClient)...)
	invApp.RegisterServices(server, pool)

	go func() {
		if err := server.Serve(lis); err != nil {
			panic(fmt.Errorf("inventory grpc serve: %w", err))
		}
	}()
	cleanups.add("inventory grpc server", func(_ context.Context) error { server.Stop(); return nil })

	// SessionForwarder автоматически пробрасывает session-uuid из контекста
	// в исходящие gRPC metadata — нужен и assembly_consumer'у для CommitParts,
	// и helper'у getStock в тесте
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(interceptor.SessionForwarder()),
	)
	if err != nil {
		panic(fmt.Errorf("inventory grpc client: %w", err))
	}
	cleanups.add("inventory grpc conn", func(_ context.Context) error { return conn.Close() })

	_ = ctx
	return conn
}

func startBufconnGRPCPayment(ctx context.Context, cleanups *cleanupStack) *grpc.ClientConn {
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer(payApp.Interceptors()...)
	payApp.RegisterServices(server)

	go func() {
		if err := server.Serve(lis); err != nil {
			panic(fmt.Errorf("payment grpc serve: %w", err))
		}
	}()
	cleanups.add("payment grpc server", func(_ context.Context) error { server.Stop(); return nil })

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(fmt.Errorf("payment grpc client: %w", err))
	}
	cleanups.add("payment grpc conn", func(_ context.Context) error { return conn.Close() })

	_ = ctx
	return conn
}

// =============================================================================
// Kafka consumers & test-assembler
// =============================================================================

func startOrderShipAssembledConsumer(
	ctx context.Context,
	cleanups *cleanupStack,
	broker string,
	pool *pgxpool.Pool,
	txManager *manager.Manager,
	invClient inventoryv1.InventoryServiceClient,
) {
	cg := mustNew(sarama.NewConsumerGroup([]string{broker}, orderGroupID, consumerConfig()))
	cleanups.add("order ship-assembled consumer group", func(_ context.Context) error { return cg.Close() })

	// ConsumerSession middleware вытаскивает session_uuid из Kafka-заголовка
	// и кладёт в контекст — иначе assembly_consumer не сможет вызвать защищённый
	// Inventory.CommitParts (тот требует session-uuid в gRPC metadata)
	wrappedConsumer := wrappedKafkaConsumer.NewConsumer(
		cg,
		[]string{shipAssembledTopic},
		wrappedKafkaConsumer.WithMiddlewares(
			kafkaMiddleware.ConsumerSession(),
		),
	)

	// Реальный код из order/internal/consumer/assembly_consumer.
	// Репозиторий и inventory-клиент берём из тех же internal-пакетов,
	// что использует прод-DI (order/internal/app/di.go)
	svc := assemblyconsumer.NewService(
		wrappedConsumer,
		orderRepoPkg.New(pool, txManager),
		inventoryClientPkg.New(invClient),
		txManager,
	)

	go func() {
		if err := svc.RunConsumer(ctx); err != nil {
			// При cancel ctx Consume вернёт ошибку — это нормально, не паникуем.
			// Логируем для диагностики, если падение настоящее
			_, _ = fmt.Fprintf(os.Stderr, "order ship-assembled consumer stopped: %v\n", err)
		}
	}()
}

// startAssemblyService поднимает реальный AssemblyService через assembly/pkg/app.
// Используется тот же код, что и в проде: consumer/order_paid → service/assembly →
// producer/ship_assembled. ConsumerSession middleware подключается внутри pkg/app —
// без неё session_uuid не пробросится в ShipAssembled-сообщение
//
// На неделе 6 build_time зашит константами 5-15 сек, поэтому таймаут ожидания
// ASSEMBLED в тесте увеличен — см. waitForOrderStatus в lifecycle_test
func startAssemblyService(ctx context.Context, cleanups *cleanupStack, broker string) {
	cg := mustNew(sarama.NewConsumerGroup([]string{broker}, assemblyGroupID, consumerConfig()))
	cleanups.add("assembly consumer group", func(_ context.Context) error { return cg.Close() })

	syncProducer := mustNew(sarama.NewSyncProducer([]string{broker}, producerConfig()))
	cleanups.add("assembly sync producer", func(_ context.Context) error { return syncProducer.Close() })

	svc := assemblyApp.New(syncProducer, cg, assemblyApp.Config{
		OrderPaidTopic:     orderPaidTopic,
		ShipAssembledTopic: shipAssembledTopic,
	})

	go func() {
		if err := svc.RunConsumer(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "assembly service stopped: %v\n", err)
		}
	}()
}

func producerConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	return cfg
}

func consumerConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	return cfg
}

// =============================================================================
// Cleanup stack — LIFO порядок shutdown без портянки defer'ов в TestMain
// =============================================================================

type cleanupStack struct {
	mu    sync.Mutex
	items []cleanupItem
}

type cleanupItem struct {
	name string
	fn   func(context.Context) error
}

func newCleanupStack() *cleanupStack {
	return &cleanupStack{}
}

func (s *cleanupStack) add(name string, fn func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, cleanupItem{name: name, fn: fn})
}

func (s *cleanupStack) run(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.items) - 1; i >= 0; i-- {
		item := s.items[i]
		if err := item.fn(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "cleanup %q failed: %v\n", item.name, err)
		}
	}
}

func mustNew[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
