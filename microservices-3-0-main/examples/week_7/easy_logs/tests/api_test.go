package tests

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/interceptor"
	applogger "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/internal/logger"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_logs/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

// Версии образов совпадают с docker-compose.yml (.env)
const (
	elasticsearchImage = "docker.elastic.co/elasticsearch/elasticsearch:9.4.0"
	otelCollectorImage = "otel/opentelemetry-collector-contrib:0.151.0"
)

var (
	lis       *bufconn.Listener
	ufoClient ufov1.UFOServiceClient
)

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	// Создаём Docker-сеть для связи Elasticsearch <-> OTel Collector
	nw, err := network.New(ctx)
	if err != nil {
		slog.Error("не удалось создать Docker сеть", "error", err)
		os.Exit(1)
	}

	// Поднимаем Elasticsearch (образ из docker-compose.yml/.env)
	esContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        elasticsearchImage,
			ExposedPorts: []string{"9200/tcp"},
			Env: map[string]string{
				"discovery.type":         "single-node",
				"xpack.security.enabled": "false",
				"cluster.name":           "es-docker-cluster",
			},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"elasticsearch"}},
			WaitingFor: wait.ForHTTP("/_cluster/health").
				WithPort("9200/tcp").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		slog.Error("не удалось запустить Elasticsearch контейнер", "error", err)
		os.Exit(1)
	}

	// Поднимаем OTel Collector (образ из docker-compose.yml/.env)
	configPath, err := filepath.Abs("../otel-collector-config.yaml")
	if err != nil {
		slog.Error("не удалось определить путь к конфигу OTel Collector", "error", err)
		os.Exit(1)
	}

	otelContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        otelCollectorImage,
			ExposedPorts: []string{"4317/tcp", "13133/tcp"},
			Cmd:          []string{"--config=/etc/otel-collector-config.yaml"},
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      configPath,
					ContainerFilePath: "/etc/otel-collector-config.yaml",
					FileMode:          0o644,
				},
			},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"otel-collector"}},
			WaitingFor: wait.ForHTTP("/").
				WithPort("13133/tcp").
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		slog.Error("не удалось запустить OTel Collector контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем динамический адрес OTel Collector для логгера
	otelHost, err := otelContainer.Host(ctx)
	if err != nil {
		slog.Error("не удалось получить хост OTel Collector", "error", err)
		os.Exit(1)
	}

	otelPort, err := otelContainer.MappedPort(ctx, "4317/tcp")
	if err != nil {
		slog.Error("не удалось получить порт OTel Collector", "error", err)
		os.Exit(1)
	}

	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", fmt.Sprintf("%s:%s", otelHost, otelPort.Port()))

	// Инициализируем логгер с OTLP (коллектор поднят через testcontainers)
	applogger.Init("info", true)

	// Настраиваем gRPC сервер через bufconn (in-process)
	lis = bufconn.Listen(bufSize)

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.LoggingInterceptor(),
		),
	)
	ufov1.RegisterUFOServiceServer(s, handler.NewUFOServer())

	go func() {
		if serveErr := s.Serve(lis); serveErr != nil {
			panic(serveErr)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать gRPC клиент", "error", err)
		os.Exit(1)
	}
	ufoClient = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	// Cleanup
	_ = applogger.Close()
	conn.Close()
	s.GracefulStop()
	_ = otelContainer.Terminate(ctx)
	_ = esContainer.Terminate(ctx)
	_ = nw.Remove(ctx)

	os.Exit(code)
}

func TestGet_Success(t *testing.T) {
	resp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: "test-uuid"})
	require.NoError(t, err)

	sighting := resp.GetSighting()
	assert.Equal(t, "test-uuid", sighting.GetUuid())
	assert.NotEmpty(t, sighting.GetInfo().GetLocation())
	assert.NotEmpty(t, sighting.GetInfo().GetObserver())
	assert.NotNil(t, sighting.GetCreatedAt())
}

func TestGet_EmptyUUID(t *testing.T) {
	_, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: ""})
	assert.Error(t, err)
}

func TestGet_MultipleCalls(t *testing.T) {
	for i := range 5 {
		uuid := "uuid-" + string(rune('a'+i))
		resp, err := ufoClient.Get(context.Background(), &ufov1.GetRequest{Uuid: uuid})
		require.NoError(t, err, "вызов %d завершился с ошибкой", i+1)
		assert.Equal(t, uuid, resp.GetSighting().GetUuid())
	}
}
