package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/interceptor"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/internal/metrics"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/easy_prometheus/pkg/proto/ufo/v1"
)

const bufSize = 1024 * 1024

// Версии образов совпадают с docker-compose.yml (.env)
const (
	prometheusImage    = "prom/prometheus:v3.10.0"
	otelCollectorImage = "otel/opentelemetry-collector-contrib:0.147.0"
)

var (
	client        ufov1.UFOServiceClient
	prometheusURL string
)

var lis *bufconn.Listener

func bufDialer(context.Context, string) (net.Conn, error) {
	return lis.Dial()
}

// TestMain поднимает Prometheus + OTel Collector через testcontainers,
// инициализирует OTel metrics и запускает gRPC сервер через bufconn
func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error

	// Создаём Docker-сеть для связи OTel Collector -> Prometheus
	nw, err := network.New(ctx)
	if err != nil {
		slog.Error("не удалось создать Docker сеть", "error", err)
		os.Exit(1)
	}

	// Поднимаем Prometheus с Remote Write API (образ из docker-compose.yml)
	promContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        prometheusImage,
			ExposedPorts: []string{"9090/tcp"},
			Cmd: []string{
				"--config.file=/dev/null",
				"--web.enable-remote-write-receiver",
			},
			Networks:       []string{nw.Name},
			NetworkAliases: map[string][]string{nw.Name: {"prometheus"}},
			WaitingFor: wait.ForHTTP("/-/healthy").
				WithPort("9090/tcp").
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		slog.Error("не удалось запустить Prometheus контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем адрес Prometheus для проверки метрик из тестов
	promHost, err := promContainer.Host(ctx)
	if err != nil {
		slog.Error("не удалось получить хост Prometheus", "error", err)
		os.Exit(1)
	}

	promPort, err := promContainer.MappedPort(ctx, "9090/tcp")
	if err != nil {
		slog.Error("не удалось получить порт Prometheus", "error", err)
		os.Exit(1)
	}

	prometheusURL = fmt.Sprintf("http://%s:%s", promHost, promPort.Port())

	// Поднимаем OTel Collector (образ из docker-compose.yml/.env)
	configPath, err := filepath.Abs("../otel-collector-config.yaml")
	if err != nil {
		slog.Error("не удалось определить путь к конфигу OTel Collector", "error", err)
		os.Exit(1)
	}

	otelContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        otelCollectorImage,
			ExposedPorts: []string{"4317/tcp"},
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
			WaitingFor: wait.ForListeningPort("4317/tcp").
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		slog.Error("не удалось запустить OTel Collector контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем динамический адрес OTel Collector для OTLP экспортера
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

	// Инициализируем OTel Meter Provider (push метрик через реальный pipeline)
	metrics.Init()

	// gRPC сервер через bufconn с otelgrpc StatsHandler и кастомным MetricsInterceptor
	lis = bufconn.Listen(bufSize)

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			interceptor.MetricsInterceptor(),
		),
	)
	ufov1.RegisterUFOServiceServer(grpcServer, handler.NewUFOServer())

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			panic(fmt.Sprintf("ошибка gRPC сервера: %v", serveErr))
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("ошибка подключения", "error", err)
		os.Exit(1)
	}

	client = ufov1.NewUFOServiceClient(conn)

	code := m.Run()

	// Cleanup
	_ = metrics.Close()
	conn.Close()
	grpcServer.GracefulStop()
	_ = otelContainer.Terminate(ctx)
	_ = promContainer.Terminate(ctx)
	_ = nw.Remove(ctx)

	os.Exit(code)
}

func TestCreate_Success(t *testing.T) {
	resp, err := client.Create(context.Background(), &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			Location:    "Москва, Красная площадь",
			Description: "Светящийся объект",
			ObservedAt:  timestamppb.Now(),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetUuid())
}

func TestCreate_InvalidArgument(t *testing.T) {
	_, err := client.Create(context.Background(), &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			Location: "",
		},
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestGet_Success(t *testing.T) {
	createResp, err := client.Create(context.Background(), &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			Location:    "Санкт-Петербург, Невский проспект",
			Description: "Треугольный объект с огнями",
			ObservedAt:  timestamppb.Now(),
		},
	})
	require.NoError(t, err)

	getResp, err := client.Get(context.Background(), &ufov1.GetRequest{Uuid: createResp.GetUuid()})
	require.NoError(t, err)
	assert.Equal(t, "Санкт-Петербург, Невский проспект", getResp.GetSighting().GetInfo().GetLocation())
	assert.Equal(t, "Треугольный объект с огнями", getResp.GetSighting().GetInfo().GetDescription())
}

func TestGet_NotFound(t *testing.T) {
	_, err := client.Get(context.Background(), &ufov1.GetRequest{Uuid: "несуществующий-uuid"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestList(t *testing.T) {
	resp, err := client.List(context.Background(), &ufov1.ListRequest{})
	require.NoError(t, err)
	assert.NotNil(t, resp.GetSightings())
}

func TestMetricsRecorded(t *testing.T) {
	// Делаем gRPC вызов, чтобы гарантированно записались метрики
	_, err := client.Create(context.Background(), &ufov1.CreateRequest{
		Info: &ufov1.SightingInfo{
			Location:    "Тест метрик",
			Description: "Проверка записи OTel метрик",
			ObservedAt:  timestamppb.Now(),
		},
	})
	require.NoError(t, err)

	// Принудительно отправляем метрики в OTel Collector → Prometheus
	require.NoError(t, metrics.Flush())

	// Ожидаем появления метрик в Prometheus с retry:
	// App ForceFlush → Collector batch (5s) → Remote Write → Prometheus
	//
	// Проверяем UpDownCounter (gauge в Prometheus) — он доходит через Remote Write
	// в отличие от monotonic counters и histograms, которые требуют
	// дополнительной конфигурации prometheusremotewrite для Prometheus v3.
	waitForPrometheusMetric(t, "ufo_service_sightings_active", "текущее количество наблюдений (UpDownCounter)")

	// Проверяем target_info — подтверждение что pipeline App → Collector → Prometheus работает
	waitForPrometheusMetric(t, "target_info", "метаданные сервиса (resource attributes)")
}

// prometheusQueryResponse — структура ответа Prometheus HTTP API
type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// waitForPrometheusMetric ожидает появления метрики в Prometheus с retry
func waitForPrometheusMetric(t *testing.T, metricName, description string) {
	t.Helper()

	require.Eventually(t, func() bool {
		return queryPrometheusMetric(t, metricName)
	}, 30*time.Second, 2*time.Second, "метрика %s (%s) не появилась в Prometheus", metricName, description)
}

// queryPrometheusMetric проверяет наличие метрики в Prometheus. Возвращает true, если метрика найдена
func queryPrometheusMetric(t *testing.T, metricName string) bool {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/query?query=%s", prometheusURL, metricName)

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var promResp prometheusQueryResponse
	if err = json.Unmarshal(body, &promResp); err != nil {
		return false
	}

	return promResp.Status == "success" && len(promResp.Data.Result) > 0
}
