package main

import (
	"cmp"
	"context"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/internal/circuitbreaker"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_circuit_breaker/pkg/proto/ufo/v1"
)

func main() {
	_ = godotenv.Load() //nolint:gosec // .env файл опционален

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	grpcHost := cmp.Or(os.Getenv("GRPC_HOST"), "localhost")
	grpcPort := cmp.Or(os.Getenv("GRPC_PORT"), "50051")
	serverAddr := net.JoinHostPort(grpcHost, grpcPort)

	// gobreaker реализует паттерн Circuit Breaker с тремя состояниями:
	//   Closed  → запросы проходят, ошибки считаются
	//   Open    → запросы мгновенно отклоняются без вызова сервера
	//   HalfOpen → пропускается ограниченное число пробных запросов для проверки восстановления
	cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		// Name — строковое имя этого circuit breaker'а
		// Gobreaker сам его никуда не отправляет — просто пробрасывает первым аргументом
		// в callback OnStateChange (см. ниже), чтобы в логах можно было различать
		// несколько circuit breaker'ов, если их больше одного
		Name: "ufo-service",

		// MaxRequests — сколько пробных запросов пропустить в состоянии HalfOpen
		// Если все MaxRequests запросов успешны — переход в Closed
		// Если хотя бы один упал — обратно в Open
		MaxRequests: 1,

		// Interval — интервал сброса счётчиков ошибок в состоянии Closed
		// Если за 10 секунд порог не достигнут — счётчики обнуляются
		// 0 означает: счётчики не сбрасываются, пока circuit breaker не перейдёт в Open
		Interval: 10 * time.Second,

		// Timeout — сколько оставаться в состоянии Open перед переходом в HalfOpen
		// После 5 секунд circuit breaker начнёт пропускать пробные запросы
		Timeout: 5 * time.Second,

		// ReadyToTrip определяет условие перехода из Closed в Open
		// Вызывается после каждой ошибки с текущими счётчиками (gobreaker.Counts):
		//
		//   Requests             — общее число запросов за текущий Interval
		//   TotalSuccesses       — сколько из них успешных
		//   TotalFailures        — сколько из них неуспешных
		//   TotalExclusions      — запросы, исключённые из подсчёта (через IsSuccessful = nil)
		//   ConsecutiveSuccesses — сколько успешных запросов подряд (сбрасывается при ошибке)
		//   ConsecutiveFailures  — сколько ошибок подряд (сбрасывается при успехе)
		//
		// Типичные стратегии:
		//   counts.ConsecutiveFailures >= N  — N ошибок подряд (простая, используем здесь)
		//   counts.TotalFailures > N         — N ошибок за Interval (абсолютный порог)
		//   float64(counts.TotalFailures)/float64(counts.Requests) > 0.5
		//                                   — процент ошибок за Interval (error rate)
		//
		// ConsecutiveFailures — самый простой вариант: если сервер вернул 3 ошибки подряд,
		// скорее всего он лежит. Не зависит от объёма трафика и не требует настройки Interval
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},

		// OnStateChange — callback при смене состояния (для логирования/мониторинга)
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Info(
				"circuit breaker: смена состояния",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
		},
	})

	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(circuitbreaker.NewClientInterceptor(cb)),
	)
	if err != nil {
		slog.Error("не удалось подключиться", "addr", serverAddr, "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := ufov1.NewUFOServiceClient(conn)
	runDemo(client)
}

// runDemo демонстрирует работу circuit breaker: успешный запрос, серия сбоев
// с открытием circuit breaker, и попытка восстановления
func runDemo(client ufov1.UFOServiceClient) {
	ctx := context.Background()

	// Успешный запрос в нормальном режиме
	slog.Info("создаём наблюдение (должно работать)...")
	resp, err := client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
		Title:   "Тестовое наблюдение",
		Content: "Содержание",
	})
	if err != nil {
		slog.Error("ошибка создания наблюдения", "error", err)
	} else {
		slog.Info("наблюдение создано", "id", resp.GetId())
	}

	// Включаем режим сбоев
	slog.Info("включаем режим сбоев на сервере...")
	_, err = client.SetFailMode(ctx, &ufov1.SetFailModeRequest{Enabled: true})
	if err != nil {
		slog.Error("ошибка включения режима сбоев", "error", err)
		return
	}

	// Делаем запросы до срабатывания circuit breaker
	for i := range 5 {
		_, err = client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
			Title: "Наблюдение при сбое",
		})
		if err != nil {
			slog.Warn("ошибка запроса", "attempt", i+1, "error", err)
		}
	}

	// Circuit breaker сейчас в состоянии Open — все запросы отклоняются мгновенно
	// Ждём Timeout (5 сек), чтобы CB перешёл в HalfOpen и пропустил пробный запрос
	slog.Info("ожидаем timeout circuit breaker (5 сек)...")
	time.Sleep(6 * time.Second) //nolint:forbidigo // демонстрационное ожидание таймаута CB

	// Выключаем режим сбоев. SetFailMode всегда возвращает OK (даже при failMode=true),
	// поэтому этот пробный запрос в HalfOpen будет успешным → CB перейдёт в Closed
	slog.Info("выключаем режим сбоев...")
	_, err = client.SetFailMode(ctx, &ufov1.SetFailModeRequest{Enabled: false})
	if err != nil {
		slog.Warn("SetFailMode через circuit breaker", "error", err)
		return
	}

	// Circuit breaker снова в состоянии Closed — запросы проходят нормально
	slog.Info("проверяем, что запросы снова проходят...")
	for i := range 3 {
		resp, createErr := client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
			Title: "Наблюдение после восстановления",
		})
		if createErr != nil {
			slog.Error("ошибка после восстановления", "attempt", i+1, "error", createErr)
		} else {
			slog.Info("запрос прошёл", "attempt", i+1, "id", resp.GetId())
		}
	}
}
