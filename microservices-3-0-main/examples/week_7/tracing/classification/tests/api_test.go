package tests

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	classificationv1API "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/api/classification/v1"
	classificationService "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/classification/internal/service/classification"
	classificationv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_7/tracing/shared/pkg/proto/classification/v1"
)

const bufSize = 1024 * 1024

var (
	lis    *bufconn.Listener
	client classificationv1.ClassificationServiceClient
)

func bufDialer(_ context.Context, _ string) (net.Conn, error) {
	return lis.Dial()
}

func TestMain(m *testing.M) {
	// DI inline — Classification (без внешних зависимостей)
	lis = bufconn.Listen(bufSize)

	svc := classificationService.NewService()
	api := classificationv1API.NewAPI(svc)

	srv := grpc.NewServer()
	classificationv1.RegisterClassificationServiceServer(srv, api)
	go srv.Serve(lis) //nolint:errcheck // Ошибка serve обрабатывается через graceful stop

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("не удалось создать gRPC клиент", "error", err)
		os.Exit(1)
	}
	client = classificationv1.NewClassificationServiceClient(conn)

	code := m.Run()

	_ = conn.Close()
	srv.Stop()
	os.Exit(code)
}

// ============ Тесты классификации по форме ============.

func TestClassifyObject_ClassicSaucer(t *testing.T) {
	testCases := []struct {
		name        string
		description string
	}{
		{"keyword диск", "Яркий диск над городом"},
		{"keyword disk", "Bright disk in the sky"},
		{"keyword тарелка", "Летающая тарелка зависла"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
				Description:     tc.description,
				Color:           "white",
				DurationSeconds: 60,
			})
			require.NoError(t, err)
			assert.Equal(t, "classic_saucer", resp.GetObjectType())
			assert.Greater(t, resp.GetConfidence(), float32(0))
			assert.NotEmpty(t, resp.GetExplanation())
		})
	}
}

func TestClassifyObject_TriangularCraft(t *testing.T) {
	testCases := []struct {
		name        string
		description string
	}{
		{"keyword треугольник", "Огромный треугольник с огнями"},
		{"keyword triangle", "Black triangle hovering"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
				Description:     tc.description,
				Color:           "white",
				DurationSeconds: 60,
			})
			require.NoError(t, err)
			assert.Equal(t, "triangular_craft", resp.GetObjectType())
			assert.Greater(t, resp.GetConfidence(), float32(0))
		})
	}
}

func TestClassifyObject_Orb(t *testing.T) {
	testCases := []struct {
		name        string
		description string
	}{
		{"keyword сфера", "Светящаяся сфера"},
		{"keyword шар", "Огненный шар в небе"},
		{"keyword sphere", "Glowing sphere"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
				Description:     tc.description,
				Color:           "white",
				DurationSeconds: 60,
			})
			require.NoError(t, err)
			assert.Equal(t, "orb", resp.GetObjectType())
			assert.Greater(t, resp.GetConfidence(), float32(0))
		})
	}
}

func TestClassifyObject_Unknown(t *testing.T) {
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "Странный объект без формы",
		Color:           "white",
		DurationSeconds: 60,
	})
	require.NoError(t, err)
	assert.Equal(t, "unknown", resp.GetObjectType())
	assert.InDelta(t, 0.52, resp.GetConfidence(), 0.01) // 0.5 base + 0.02 white
}

// ============ Тесты корректировки по цвету ============.

func TestClassifyObject_ColorAdjustment(t *testing.T) {
	baseDescription := "Яркий диск" // classic_saucer, base confidence 0.9

	testCases := []struct {
		name               string
		color              string
		expectedMinConf    float32
		expectedExtraInExp string
	}{
		{"green bonus +0.1", "green", 1.0, "Зелёное свечение"},
		{"зеленый bonus +0.1", "зеленый", 1.0, "Зелёное свечение"},
		{"red bonus +0.05", "red", 0.95, "Красный цвет"},
		{"красный bonus +0.05", "красный", 0.95, "Красный цвет"},
		{"white bonus +0.02", "white", 0.92, "Белое свечение"},
		{"белый bonus +0.02", "белый", 0.92, "Белое свечение"},
		{"unknown color no bonus", "purple", 0.9, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
				Description:     baseDescription,
				Color:           tc.color,
				DurationSeconds: 60, // neutral duration
			})
			require.NoError(t, err)
			assert.InDelta(t, tc.expectedMinConf, resp.GetConfidence(), 0.01)
			if tc.expectedExtraInExp != "" {
				assert.Contains(t, resp.GetExplanation(), tc.expectedExtraInExp)
			}
		})
	}
}

// ============ Тесты корректировки по длительности ============.

func TestClassifyObject_DurationAdjustment(t *testing.T) {
	baseDescription := "Яркий диск" // classic_saucer, base confidence 0.9

	testCases := []struct {
		name           string
		duration       int32
		expectedConf   float32
		expectedInExpl string
	}{
		{"long observation > 600s", 700, 1.0, "Длительное наблюдение"},
		{"short observation < 30s", 10, 0.8, "Кратковременное наблюдение"},
		{"neutral duration 60s", 60, 0.9, ""},
		{"boundary 600s neutral", 600, 0.9, ""},
		{"boundary 30s neutral", 30, 0.9, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
				Description:     baseDescription,
				Color:           "purple", // unknown color, no bonus
				DurationSeconds: tc.duration,
			})
			require.NoError(t, err)
			assert.InDelta(t, tc.expectedConf, resp.GetConfidence(), 0.01)
			if tc.expectedInExpl != "" {
				assert.Contains(t, resp.GetExplanation(), tc.expectedInExpl)
			}
		})
	}
}

// ============ Тест confidence clamping ============.

func TestClassifyObject_ConfidenceClamped(t *testing.T) {
	// green (+0.1) + long duration (+0.1) + classic_saucer (0.9) = 1.1 -> clamped to 1.0
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "Яркий диск",
		Color:           "green",
		DurationSeconds: 700,
	})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, resp.GetConfidence(), 0.001, "confidence должен быть ограничен 1.0")
}

func TestClassifyObject_ConfidenceNotNegative(t *testing.T) {
	// unknown (0.5) + short duration (-0.1) = 0.4 — still positive
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "Нечто непонятное",
		Color:           "purple",
		DurationSeconds: 5,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.GetConfidence(), float32(0), "confidence не должен быть отрицательным")
}

// ============ Тест пустых значений ============.

func TestClassifyObject_EmptyFields(t *testing.T) {
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "",
		Color:           "",
		DurationSeconds: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, "unknown", resp.GetObjectType())
	assert.Greater(t, resp.GetConfidence(), float32(0))
}

// ============ Тест комбинации всех факторов ============.

func TestClassifyObject_CombinedFactors(t *testing.T) {
	// triangular_craft (0.8) + red (+0.05) + long duration (+0.1) = 0.95
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "Чёрный треугольник с огнями",
		Color:           "red",
		DurationSeconds: 700,
	})
	require.NoError(t, err)
	assert.Equal(t, "triangular_craft", resp.GetObjectType())
	assert.InDelta(t, 0.95, resp.GetConfidence(), 0.01)
	assert.Contains(t, resp.GetExplanation(), "Треугольная форма")
	assert.Contains(t, resp.GetExplanation(), "Красный цвет")
	assert.Contains(t, resp.GetExplanation(), "Длительное наблюдение")
}

func TestClassifyObject_CaseInsensitive(t *testing.T) {
	// Description to lowercase — "ДИСК" should match "диск"
	resp, err := client.ClassifyObject(context.Background(), &classificationv1.ClassifyObjectRequest{
		Description:     "Яркий ДИСК в небе",
		Color:           "GREEN",
		DurationSeconds: 60,
	})
	require.NoError(t, err)
	assert.Equal(t, "classic_saucer", resp.GetObjectType())
}
