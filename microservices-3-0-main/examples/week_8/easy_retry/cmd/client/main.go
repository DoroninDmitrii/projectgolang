package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_8/easy_retry/pkg/proto/ufo/v1"
)

const serverAddr = "localhost:50051"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Настраиваем политику повторных попыток (retry):
	//
	// WithMax(3) — максимум 3 попытки (1 основная + 2 повтора)
	//
	// WithBackoff — задержка между повторами. BackoffExponentialWithJitter(100ms, 0.2) означает:
	//   - Первый аргумент (100ms) — начальная задержка
	//   - Каждый следующий повтор удваивает задержку: 100ms → 200ms → 400ms → ...
	//   - Второй аргумент (0.2) — jitter, случайный разброс ±20% от задержки,
	//     чтобы клиенты не повторяли запросы одновременно и не создавали
	//     "волну" нагрузки на сервер
	//
	// WithCodes — коды ошибок, при которых имеет смысл повторять запрос
	//   Unavailable — сервер временно недоступен (перезапуск, перегрузка)
	//   ResourceExhausted — сервер перегружен (например, rate limit)
	//   Бизнес-ошибки (InvalidArgument, NotFound и др.) повторять бесполезно —
	//   результат будет тот же
	// OnRetryCallback вызывается ПЕРЕД каждой повторной попыткой (после исходного провала).
	// attempt — порядковый номер ретрая, начиная с 1 (1 = первый ретрай, 2 = второй и т.д.),
	// err — ошибка, из-за которой делаем ретрай.
	// Без этого callback ретраи невидимы для клиента: видишь "создаём" → "создано",
	// а что между ними было 2 неудачи и backoff — узнаёшь только из логов сервера.
	onRetry := func(_ context.Context, attempt uint, err error) {
		slog.Warn(
			"повторяю запрос",
			"retry", attempt,
			"error", err.Error(),
		)
	}

	retryOpts := []grpcretry.CallOption{
		grpcretry.WithMax(3),
		grpcretry.WithBackoff(grpcretry.BackoffExponentialWithJitter(100*time.Millisecond, 0.2)),
		grpcretry.WithCodes(codes.Unavailable, codes.ResourceExhausted),
		grpcretry.WithOnRetryCallback(onRetry),
	}

	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcretry.UnaryClientInterceptor(retryOpts...)),
	)
	if err != nil {
		slog.Error("не удалось подключиться", "addr", serverAddr, "error", err)
		return
	}
	defer conn.Close()

	client := ufov1.NewUFOServiceClient(conn)
	ctx := context.Background()

	// === Сценарий 1: транзиентная ошибка → retry помогает ===
	// Сервер сбоит первые 2 запроса с Unavailable. Ожидаем 2 предупреждения от
	// OnRetryCallback и итоговое "создано". Общее время вызова будет ~300ms+
	// из-за backoff (100ms + 200ms), хотя сама обработка занимает миллисекунды.
	slog.Info("=== Сценарий 1: транзиентный сбой → retry ===")

	start := time.Now()
	resp, err := client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
		Title:   "Наблюдение с retry",
		Content: "Это наблюдение было создано после автоматических retry",
	})
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		slog.Error(
			"не удалось создать наблюдение после всех попыток",
			"error", err,
			"elapsed", elapsed.String(),
		)
		return
	}
	slog.Info(
		"наблюдение создано",
		"id", resp.GetId(),
		"elapsed", elapsed.String(),
	)

	// === Сценарий 2: бизнес-ошибка → retry НЕ срабатывает ===
	// Пустой Title → InvalidArgument. Этого кода нет в WithCodes,
	// поэтому клиент возвращает ошибку сразу, без повторов и backoff.
	// Elapsed должен быть единицы миллисекунд, OnRetryCallback не вызовется.
	slog.Info("=== Сценарий 2: бизнес-ошибка → без retry ===")

	start = time.Now()
	_, err = client.CreateUfo(ctx, &ufov1.CreateUfoRequest{
		Title:   "", // пустой заголовок -> InvalidArgument
		Content: "невалидный запрос",
	})
	elapsed = time.Since(start).Round(time.Millisecond)
	if err == nil {
		slog.Error("ожидалась ошибка валидации, но запрос прошёл")
		return
	}
	slog.Info(
		"получили ошибку сразу, без повторов (как и ожидалось)",
		"error", err.Error(),
		"elapsed", elapsed.String(),
	)
}
