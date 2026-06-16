package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/workspace/http/pkg/handler"
)

const (
	httpPort = "8080"

	// Таймауты для HTTP-сервера
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("ошибка запуска сервера", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Создаем хранилище для данных о погоде
	storage := handler.NewWeatherStorage()

	// Создаем обработчик API погоды
	weatherHandler := handler.NewWeatherHandler(storage)

	// Создаем и настраиваем роутер
	r, err := handler.SetupRouter(weatherHandler)
	if err != nil {
		return fmt.Errorf("создать роутер: %w", err)
	}

	// Запускаем HTTP-сервер с таймаутами для защиты от атак
	// Подробное описание всех параметров: см. week_1/HTTP_SERVER.md
	server := &http.Server{
		Addr:              net.JoinHostPort("localhost", httpPort),
		Handler:           r,
		ReadHeaderTimeout: readHeaderTimeout, // Защита от Slowloris атаки
		ReadTimeout:       readTimeout,       // Лимит на чтение всего запроса
		WriteTimeout:      writeTimeout,      // Лимит на запись ответа
		IdleTimeout:       idleTimeout,       // Таймаут keep-alive соединений
	}

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении сервера
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем сервер в отдельной горутине
	go func() {
		slog.Info("🚀 HTTP-сервер запущен на порту", "port", httpPort)
		listenErr := server.ListenAndServe()
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("❌ ошибка запуска сервера", "error", listenErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём сигнал от ОС или падение сервера
	<-ctx.Done()
	slog.Info("🛑 завершение работы сервера...")

	// Создаем контекст с таймаутом для остановки сервера
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	err = server.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("❌ ошибка при остановке сервера", "error", err)
	}

	slog.Info("✅ сервер остановлен")
	return nil
}
