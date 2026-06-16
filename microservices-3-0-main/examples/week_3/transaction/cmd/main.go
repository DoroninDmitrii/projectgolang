package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/errors"
	accountRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/account"
	accountService "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/service/account"
)

const (
	aliceUUID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001"
	bobUUID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002"
	karlUUID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003"

	transferAmount = 10000 // 100₽ в копейках
)

func main() {
	if err := run(); err != nil {
		slog.Error("ошибка выполнения", "error", err)
		os.Exit(1)
	}
}

//nolint:funlen // учебный демо-сценарий
func run() error {
	ctx := context.Background()

	// Загружаем переменные окружения из .env
	err := godotenv.Load(".env")
	if err != nil {
		return err
	}

	dbURI := os.Getenv("DB_URI")

	// 1. Создаём пул соединений к PostgreSQL
	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		return err
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		return err
	}

	slog.Info("подключение к PostgreSQL установлено")

	// 2. Создаём Transaction Manager
	// trmpgx.NewDefaultFactory(pool) — фабрика, которая умеет создавать pgx-транзакции
	// из пула соединений. manager.New оборачивает её в менеджер с методом Do()
	//
	// Как это работает вместе:
	//   txManager.Do(ctx, fn) → фабрика берёт conn из pool → BEGIN →
	//   → кладёт pgx.Tx в новый ctx → вызывает fn(ctxWithTx) →
	//   → fn возвращает nil → COMMIT / fn возвращает err → ROLLBACK
	txManager, err := manager.New(trmpgx.NewDefaultFactory(pool))
	if err != nil {
		return err
	}

	// 3. Wire: репозиторий → сервис
	// repo получает pool — для прямых запросов и как fallback в getter.DefaultTrOrDB
	// svc получает txManager — для оборачивания бизнес-операций в транзакции,
	// и repo — для выполнения запросов к БД внутри этих транзакций
	repo := accountRepo.New(pool)
	svc := accountService.New(txManager, repo)

	// 4. Демо: показываем начальные балансы (CollectRows + ANY)
	slog.Info("--- Начальные балансы ---")

	allUUIDs := []string{aliceUUID, bobUUID, karlUUID}

	accounts, err := repo.List(ctx, allUUIDs)
	if err != nil {
		return err
	}

	for _, acc := range accounts {
		desc := "<нет>"
		if acc.Description != nil {
			desc = *acc.Description
		}

		slog.Info(
			"счёт",
			"owner", acc.Owner,
			"balance", acc.Balance,
			"description", desc,
		)
	}

	// 5. Демо: успешный перевод от Алисы к Бобу (100₽)
	slog.Info("--- Перевод 100₽ от Алисы к Бобу ---")

	err = svc.Transfer(ctx, aliceUUID, bobUUID, transferAmount)
	if err != nil {
		return err
	}

	slog.Info("перевод выполнен успешно")

	// 6. Показываем обновлённые балансы
	slog.Info("--- Балансы после перевода ---")

	accounts, err = repo.List(ctx, allUUIDs)
	if err != nil {
		return err
	}

	for _, acc := range accounts {
		slog.Info(
			"счёт",
			"owner", acc.Owner,
			"balance", acc.Balance,
		)
	}

	// 7. Демо: перевод с недостаточным балансом (rollback)
	slog.Info("--- Попытка перевода 99999999 копеек от Карла к Алисе ---")

	err = svc.Transfer(ctx, karlUUID, aliceUUID, 99999999)
	if err != nil {
		if errors.Is(err, errs.ErrInsufficientFunds) {
			slog.Info("перевод отклонён: недостаточно средств (транзакция откачена)")
		} else {
			return err
		}
	}

	// 8. Показываем балансы после отклонённого перевода (должны быть без изменений)
	slog.Info("--- Балансы после отклонённого перевода (без изменений) ---")

	accounts, err = repo.List(ctx, allUUIDs)
	if err != nil {
		return err
	}

	for _, acc := range accounts {
		slog.Info(
			"счёт",
			"owner", acc.Owner,
			"balance", acc.Balance,
		)
	}

	return nil
}
