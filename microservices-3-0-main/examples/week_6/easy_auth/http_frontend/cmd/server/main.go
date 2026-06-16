package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/http_frontend/pkg/handler"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/http_frontend/pkg/middleware"
	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/proto/ufo/v1"
)

const (
	// Адреса серверов
	grpcBackendAddress = "localhost:50051"
	httpAddress        = ":8080"

	// HTTP таймауты
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 15 * time.Second
	httpIdleTimeout       = 60 * time.Second
	httpShutdownTimeout   = 5 * time.Second

	// gRPC клиент keepalive параметры
	grpcKeepaliveTime    = 5 * time.Minute
	grpcKeepaliveTimeout = 1 * time.Second
)

func main() {
	// Создаём gRPC соединение — один раз при старте
	conn, err := grpc.NewClient(
		grpcBackendAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                grpcKeepaliveTime,
			Timeout:             grpcKeepaliveTimeout,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		slog.Error("ошибка подключения к gRPC бэкенду", "error", err)
		return
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			slog.Error("ошибка закрытия gRPC соединения", "error", closeErr)
		}
	}()

	ufoClient := ufov1.NewUFOServiceClient(conn)
	h := handler.New(ufoClient)

	// Chi router с middleware
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Все маршруты /api/ защищены auth middleware
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.HTTPAuth)

		r.Get("/sightings", h.ListSightings)
		r.Post("/sightings", h.CreateSighting)
		r.Get("/sightings/{id}", h.GetSighting)
	})

	httpServer := &http.Server{
		Addr:              httpAddress,
		Handler:           r,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		slog.Info("🌐 HTTP Frontend запущен", "address", httpAddress)
		slog.Info("пример: curl -H 'Authorization: Bearer token-admin-123' http://localhost:8080/api/v1/sightings")
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("ошибка запуска HTTP сервера", "error", serveErr)
			cancel()
		}
	}()

	// Ждём либо сигнал от ОС, либо падение сервера
	<-ctx.Done()
	slog.Info("🛑 остановка HTTP сервера")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelShutdown()
	if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("ошибка остановки HTTP сервера", "error", shutdownErr)
	}
	slog.Info("✅ HTTP сервер остановлен")
}
