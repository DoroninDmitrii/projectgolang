package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// User — структура для парсинга JSON-запроса
type User struct {
	Username string `json:"username"` // имя пользователя
	Email    string `json:"email"`    // email пользователя
}

const (
	// Таймауты для HTTP-сервера
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
	dbPingTimeout     = 5 * time.Second
	dbQueryTimeout    = 5 * time.Second
)

// setupHandler создаёт HTTP mux с зарегистрированными обработчиками
func setupHandler(pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", createUserHandler(pool))

	return mux
}

// createUserHandler — обработчик POST-запросов для создания нового пользователя
func createUserHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, что метод запроса — POST
		if r.Method != http.MethodPost {
			http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		var user User
		// Парсим JSON-тело запроса в структуру User
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			http.Error(w, "Некорректный JSON", http.StatusBadRequest)
			return
		}

		// Проверяем, что оба обязательных поля присутствуют
		if user.Username == "" || user.Email == "" {
			http.Error(w, "Поля username и email обязательны", http.StatusBadRequest)
			return
		}

		// Контекст с таймаутом для выполнения запроса к базе
		ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
		defer cancel()

		// SQL-запрос на вставку данных в таблицу users
		query := `INSERT INTO users (username, email) VALUES ($1, $2)`
		_, err := pool.Exec(ctx, query, user.Username, user.Email)
		if err != nil {
			slog.Error("ошибка вставки в базу", "error", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
			return
		}

		// Возвращаем успешный ответ клиенту
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("Пользователь успешно создан\n")) //nolint:gosec // Ошибка записи в ResponseWriter игнорируется намеренно
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Адрес HTTP-сервера из переменной окружения
	httpAddress := os.Getenv("HTTP_ADDRESS")

	// Строка подключения к Postgres
	dbURI := os.Getenv("DB_URI")

	// Инициализируем пул соединений к базе данных Postgres
	var err error

	db, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("ошибка подключения к базе данных", "error", err)
		return
	}
	// Закрываем пул соединений при завершении работы приложения
	defer db.Close()

	// Выставляем таймаут для проверки подключения к базе
	pingCtx, cancelPing := context.WithTimeout(ctx, dbPingTimeout)
	defer cancelPing()

	// Проверяем, что соединение с базой установлено
	err = db.Ping(pingCtx)
	if err != nil {
		slog.Error("база данных недоступна", "error", err)
		return
	}

	// Создаём HTTP handler с зарегистрированными обработчиками
	handler := setupHandler(db)

	// Запускаем HTTP-сервер с таймаутами для защиты от атак
	srv := &http.Server{
		Addr:              httpAddress,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Запускаем сервер в отдельной горутине
	go func() {
		slog.Info("🚀 HTTP сервер запущен", "address", httpAddress)
		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("ошибка запуска сервера", "error", listenErr)
			cancel()
		}
	}()

	// Ждём сигнал ОС или падение сервера
	<-ctx.Done()

	slog.Info("🛑 завершение работы сервера...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		slog.Error("ошибка при остановке сервера", "error", err)
	}

	slog.Info("✅ сервер остановлен")
}
