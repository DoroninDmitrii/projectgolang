package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/http_frontend/pkg/handler"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/http_grpc_integration/shared/pkg/proto/ufo/v1"
)

const (
	// Адреса серверов
	grpcBackendAddress = "localhost:50051"
	httpAddress        = ":8080"

	// HTTP таймауты
	httpReadHeaderTimeout = 5 * time.Second  // Защита от Slowloris атаки
	httpReadTimeout       = 15 * time.Second // Лимит на чтение всего запроса
	httpWriteTimeout      = 15 * time.Second // Лимит на запись ответа
	httpIdleTimeout       = 60 * time.Second // Таймаут keep-alive соединений
	httpShutdownTimeout   = 5 * time.Second  // Таймаут graceful shutdown

	// gRPC клиент keepalive параметры
	grpcKeepaliveTime    = 5 * time.Minute // Интервал ping'ов для обнаружения мёртвого сервера
	grpcKeepaliveTimeout = 1 * time.Second // Таймаут ожидания pong
)

func main() {
	// СОЗДАНИЕ gRPC СОЕДИНЕНИЯ
	// Это делается один раз при старте сервиса!
	conn, err := grpc.NewClient(
		grpcBackendAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                grpcKeepaliveTime,    // Интервал ping'ов
			Timeout:             grpcKeepaliveTimeout, // Таймаут ожидания pong
			PermitWithoutStream: true,                 // Пинговать даже без активных RPC
		}),
	)
	if err != nil {
		slog.Error("ошибка подключения к gRPC бэкенду", "error", err)
		return
	}
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil {
			slog.Error("ошибка закрытия gRPC соединения", "error", closeErr)
		}
	}()

	// Создаём gRPC клиент
	ufoClient := ufov1.NewUFOServiceClient(conn)

	// Создаём handler с gRPC клиентом как зависимостью
	h := handler.New(ufoClient)

	// Настройка роутинга
	mux := handler.SetupMux(h)

	// Создаем HTTP сервер с таймаутами для защиты от атак
	// Подробное описание всех параметров: см. week_1/HTTP_SERVER.md
	httpServer := &http.Server{
		Addr:              httpAddress,
		Handler:           mux,
		ReadHeaderTimeout: httpReadHeaderTimeout, // Защита от Slowloris атаки
		ReadTimeout:       httpReadTimeout,       // Лимит на чтение всего запроса
		WriteTimeout:      httpWriteTimeout,      // Лимит на запись ответа
		IdleTimeout:       httpIdleTimeout,       // Таймаут keep-alive соединений
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем HTTP сервер в горутине
	go func() {
		slog.Info("🌐 HTTP Frontend запущен", "address", httpAddress)
		slog.Info("примеры запросов:")
		slog.Info("  curl http://localhost:8080/api/v1/sightings")
		slog.Info("  curl http://localhost:8080/api/v1/sightings/sighting-001")
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("ошибка запуска HTTP сервера", "error", serveErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём сигнал от ОС или падение сервера
	<-ctx.Done()
	slog.Info("🛑 остановка HTTP сервера")

	// Аккуратно останавливаем HTTP сервер
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelShutdown()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("ошибка остановки HTTP сервера", "error", shutdownErr)
	}
	slog.Info("✅ HTTP сервер остановлен")
}
