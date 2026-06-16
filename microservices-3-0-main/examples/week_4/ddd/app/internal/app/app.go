package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/config"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/platform/pkg/closer"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/platform/pkg/logger"
)

const (
	shutdownTimeout   = 5 * time.Second
	readHeaderTimeout = 5 * time.Second
)

// App — корневая структура приложения, управляющая жизненным циклом всех компонентов
type App struct {
	diContainer *diContainer
	router      *chi.Mux
}

// New создаёт и инициализирует приложение
func New(ctx context.Context) *App {
	a := &App{}

	a.initDeps(ctx)

	return a
}

// initDeps последовательно инициализирует все зависимости приложения
func (a *App) initDeps(ctx context.Context) {
	inits := []func(context.Context){
		a.initDI,
		a.initLogger,
		a.initRouter,
	}

	for _, f := range inits {
		f(ctx)
	}
}

// initDI создаёт DI-контейнер
func (a *App) initDI(_ context.Context) {
	a.diContainer = &diContainer{}
}

// initLogger настраивает глобальный slog с уровнем из конфига
func (a *App) initLogger(_ context.Context) {
	logger.Init(config.AppConfig().Logger.Level)
}

// initRouter создаёт chi-роутер и регистрирует маршруты
func (a *App) initRouter(ctx context.Context) {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	h := a.diContainer.PCBuilderHandler(ctx)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/builds", h.CreateBuild)
		r.Post("/builds/{uuid}/cancel", h.CancelBuild)
	})

	a.router = r
}

// Run запускает HTTP-сервер и управляет жизненным циклом приложения
//
// Алгоритм:
//  1. Запускает HTTP-сервер в отдельной горутине
//  2. Блокируется до получения SIGINT/SIGTERM
//  3. Выполняет graceful shutdown: останавливает сервер, затем закрывает остальные ресурсы
func (a *App) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	srv := &http.Server{
		Addr:              config.AppConfig().HTTP.Addr(),
		Handler:           a.router,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Запускаем HTTP-сервер в отдельной горутине
	go func() {
		slog.Info("🌐 HTTP-сервер запущен", "addr", srv.Addr)

		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("ошибка HTTP-сервера", "error", listenErr)
			cancel()
		}
	}()

	// Блокируемся до получения SIGINT/SIGTERM или падения сервера
	<-ctx.Done()

	a.gracefulShutdown(cancel, srv)

	return nil
}

// gracefulShutdown корректно останавливает все компоненты приложения
//
// Порядок работы:
//  1. cancel() снимает перехват сигналов (вызывает signal.Stop внутри)
//     Контекст к этому моменту уже отменён. Это нужно, чтобы повторный Ctrl+C
//     не перехватывался, а завершил процесс принудительно (поведение ОС по умолчанию)
//  2. Останавливаем HTTP-сервер — перестаём принимать новые запросы, дожидаемся текущих
//  3. Закрываем остальные ресурсы через closer (БД, и т.д.)
func (a *App) gracefulShutdown(cancel context.CancelFunc, srv *http.Server) {
	cancel()

	slog.Info("получен сигнал завершения, начинаем graceful shutdown")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("ошибка остановки HTTP-сервера", "error", err)
	}

	if err := closer.CloseAll(shutdownCtx); err != nil {
		slog.Error("ошибка при завершении работы", "error", err)
	}
}
