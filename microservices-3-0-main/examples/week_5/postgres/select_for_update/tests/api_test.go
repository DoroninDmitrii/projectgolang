package tests

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"sync"
	"testing"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx как database/sql драйвер для goose
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/errors"
	accountRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/repository/account"
	accountService "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/service/account"
)

const (
	aliceUUID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001"
	bobUUID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002"
	karlUUID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003"
)

var (
	pool *pgxpool.Pool
	svc  *accountService.Service
)

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции и собирает слои приложения
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из .env
	_ = godotenv.Load("../.env")

	// Поднимаем PostgreSQL через testcontainers
	container, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase(os.Getenv("POSTGRES_DB")),
		postgres.WithUsername(os.Getenv("POSTGRES_USER")),
		postgres.WithPassword(os.Getenv("POSTGRES_PASSWORD")),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	dbURI, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	sqlDB, err := sql.Open("pgx", dbURI)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}

	if err = goose.Up(sqlDB, "../migrations"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	sqlDB.Close()

	// Подключаемся к PostgreSQL через pgxpool
	pool, err = pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// Собираем слои приложения (repository → service)
	txManager, err := manager.New(trmpgx.NewDefaultFactory(pool))
	if err != nil {
		slog.Error("не удалось создать transaction manager", "error", err)
		os.Exit(1)
	}

	repo := accountRepo.New(pool)
	svc = accountService.New(txManager, repo)

	code := m.Run()

	pool.Close()
	_ = container.Terminate(ctx)

	os.Exit(code)
}

// resetAccounts сбрасывает балансы к начальным значениям из seed-миграции
func resetAccounts(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		UPDATE accounts SET
			balance = CASE uuid
				WHEN 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001' THEN 1000000
				WHEN 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002' THEN 500000
				WHEN 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003' THEN 250000
			END,
			updated_at = NULL
		WHERE uuid IN (
			'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001',
			'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002',
			'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003'
		)
	`)
	require.NoError(t, err, "не удалось сбросить балансы")
}

func getBalance(t *testing.T, uuid string) int64 {
	t.Helper()

	var balance int64
	err := pool.QueryRow(context.Background(),
		"SELECT balance FROM accounts WHERE uuid = $1", uuid).Scan(&balance)
	require.NoError(t, err, "не удалось получить баланс")

	return balance
}

func totalBalance(t *testing.T) int64 {
	t.Helper()

	return getBalance(t, aliceUUID) + getBalance(t, bobUUID) + getBalance(t, karlUUID)
}

// TestTransfer_ConcurrentSafe проверяет, что безопасный Transfer сохраняет
// суммарный баланс при параллельных переводах
func TestTransfer_ConcurrentSafe(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()
	totalBefore := totalBalance(t)

	const (
		workers = 50
		amount  = 1000
	)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Transfer(ctx, aliceUUID, bobUUID, amount)
		}()
	}
	wg.Wait()

	// Суммарный баланс должен сохраниться
	totalAfter := totalBalance(t)
	assert.Equal(t, totalBefore, totalAfter, "суммарный баланс не должен измениться")

	// Баланс Алисы должен быть ровно на workers*amount меньше
	aliceBalance := getBalance(t, aliceUUID)
	assert.Equal(t, int64(1000000-workers*amount), aliceBalance, "баланс Алисы должен быть корректным")

	// Баланс Боба должен быть ровно на workers*amount больше
	bobBalance := getBalance(t, bobUUID)
	assert.Equal(t, int64(500000+workers*amount), bobBalance, "баланс Боба должен быть корректным")
}

// TestTransfer_DeadlockPrevention проверяет, что встречные переводы A→B и B→A
// не приводят к deadlock'у благодаря сортировке UUID
func TestTransfer_DeadlockPrevention(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()
	totalBefore := totalBalance(t)

	const (
		workers = 20
		amount  = 100
	)

	var wg sync.WaitGroup
	for range workers {
		// A → B
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Transfer(ctx, aliceUUID, bobUUID, amount)
		}()

		// B → A (встречный перевод)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Transfer(ctx, bobUUID, aliceUUID, amount)
		}()
	}
	wg.Wait()

	// Суммарный баланс должен сохраниться
	totalAfter := totalBalance(t)
	assert.Equal(t, totalBefore, totalAfter, "суммарный баланс не должен измениться после встречных переводов")
}

// TestTransfer_InsufficientFunds проверяет откат при недостатке средств
func TestTransfer_InsufficientFunds(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()

	err := svc.Transfer(ctx, karlUUID, aliceUUID, 99999999)
	require.ErrorIs(t, err, errs.ErrInsufficientFunds, "ошибка должна быть ErrInsufficientFunds")

	// Балансы не должны измениться
	assert.Equal(t, int64(250000), getBalance(t, karlUUID), "баланс Карла не должен измениться")
	assert.Equal(t, int64(1000000), getBalance(t, aliceUUID), "баланс Алисы не должен измениться")
}
