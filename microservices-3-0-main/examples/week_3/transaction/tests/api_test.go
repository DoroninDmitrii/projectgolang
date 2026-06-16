package tests

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/errors"
	accountRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/account"
	accountService "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/service/account"
)

const (
	aliceUUID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001"
	bobUUID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002"
	karlUUID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003"

	migrationsDir = "../migrations"
)

var (
	pool  *pgxpool.Pool
	sqlDB *sql.DB
	svc   *accountService.Service
	repo  accountService.AccountRepository
)

// TestMain поднимает PostgreSQL через testcontainers, применяет миграции и запускает тесты
func TestMain(m *testing.M) {
	// Загружаем переменные окружения из .env (креды для PostgreSQL)
	_ = godotenv.Load("../.env")

	pgUser := os.Getenv("POSTGRES_USER")
	pgPassword := os.Getenv("POSTGRES_PASSWORD")

	ctx := context.Background()

	// Поднимаем PostgreSQL в контейнере на случайном порту
	pgContainer, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase("postgres"),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем строку подключения с динамическим портом
	dbURI, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// database/sql соединение для goose
	sqlDB, err = sql.Open("pgx", dbURI)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}

	if err = goose.Up(sqlDB, migrationsDir); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	// pgxpool для приложения и тестов
	pool, err = pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// Создаём Transaction Manager
	txManager, err := manager.New(trmpgx.NewDefaultFactory(pool))
	if err != nil {
		slog.Error("создание transaction manager", "error", err)
		os.Exit(1)
	}

	repo = accountRepo.New(pool)
	svc = accountService.New(txManager, repo)

	code := m.Run()

	pool.Close()
	sqlDB.Close()

	if termErr := pgContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

// resetAccounts откатывает и заново применяет seed-миграцию (00002),
// чтобы вернуть данные к начальному состоянию
func resetAccounts(t *testing.T) {
	t.Helper()

	// Откатываем seed-миграцию (goose Down для 00002 делает DELETE)
	err := goose.Down(sqlDB, migrationsDir)
	require.NoError(t, err, "не удалось откатить seed-миграцию")

	// Применяем заново (goose Up для 00002 делает INSERT)
	err = goose.Up(sqlDB, migrationsDir)
	require.NoError(t, err, "не удалось применить seed-миграцию")
}

// getBalance возвращает баланс счёта по UUID
func getBalance(t *testing.T, uuid string) int64 {
	t.Helper()

	var balance int64
	err := pool.QueryRow(context.Background(),
		"SELECT balance FROM accounts WHERE uuid = $1", uuid).Scan(&balance)
	require.NoError(t, err, "не удалось получить баланс")

	return balance
}

// TestTransfer_Success проверяет успешный перевод между счетами
func TestTransfer_Success(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()
	amount := int64(10000) // 100₽

	// Балансы до перевода
	aliceBefore := getBalance(t, aliceUUID)
	bobBefore := getBalance(t, bobUUID)

	// Выполняем перевод
	err := svc.Transfer(ctx, aliceUUID, bobUUID, amount)
	require.NoError(t, err, "перевод должен пройти успешно")

	// Проверяем балансы после перевода
	aliceAfter := getBalance(t, aliceUUID)
	bobAfter := getBalance(t, bobUUID)

	assert.Equal(t, aliceBefore-amount, aliceAfter, "баланс Алисы должен уменьшиться на сумму перевода")
	assert.Equal(t, bobBefore+amount, bobAfter, "баланс Боба должен увеличиться на сумму перевода")
}

// TestTransfer_InsufficientFunds проверяет откат транзакции при недостатке средств
func TestTransfer_InsufficientFunds(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()
	amount := int64(99999999) // больше, чем на счёте Карла

	// Балансы до попытки перевода
	karlBefore := getBalance(t, karlUUID)
	aliceBefore := getBalance(t, aliceUUID)

	// Пытаемся перевести больше, чем есть
	err := svc.Transfer(ctx, karlUUID, aliceUUID, amount)
	require.ErrorIs(t, err, errs.ErrInsufficientFunds, "ошибка должна быть ErrInsufficientFunds")

	// Балансы не должны измениться (транзакция откачена)
	karlAfter := getBalance(t, karlUUID)
	aliceAfter := getBalance(t, aliceUUID)

	assert.Equal(t, karlBefore, karlAfter, "баланс Карла не должен измениться")
	assert.Equal(t, aliceBefore, aliceAfter, "баланс Алисы не должен измениться")
}

// TestTransfer_AccountNotFound проверяет ошибку при переводе на несуществующий счёт
func TestTransfer_AccountNotFound(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()
	nonExistentUUID := "00000000-0000-0000-0000-000000000000"

	// Баланс Алисы до попытки перевода
	aliceBefore := getBalance(t, aliceUUID)

	err := svc.Transfer(ctx, aliceUUID, nonExistentUUID, 100)
	require.ErrorIs(t, err, errs.ErrAccountNotFound, "ошибка должна быть ErrAccountNotFound")

	// Баланс Алисы не должен измениться (транзакция откачена)
	aliceAfter := getBalance(t, aliceUUID)
	assert.Equal(t, aliceBefore, aliceAfter, "баланс Алисы не должен измениться")
}

// TestList_CollectRows проверяет получение списка счетов через CollectRows + ANY($1)
func TestList_CollectRows(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()

	// Запрашиваем два счёта из трёх
	accounts, err := repo.List(ctx, []string{aliceUUID, karlUUID})
	require.NoError(t, err, "List не должен возвращать ошибку")
	assert.Len(t, accounts, 2, "должно быть 2 счёта")

	// Проверяем, что вернулись правильные счета
	owners := make(map[string]bool)
	for _, acc := range accounts {
		owners[acc.Owner] = true
	}

	assert.True(t, owners["Алиса"], "Алиса должна быть в списке")
	assert.True(t, owners["Карл"], "Карл должен быть в списке")
}

// TestGet_NullableFields проверяет корректное чтение nullable полей
func TestGet_NullableFields(t *testing.T) {
	resetAccounts(t)

	ctx := context.Background()

	// Алиса имеет description
	alice, err := repo.Get(ctx, aliceUUID)
	require.NoError(t, err)
	require.NotNil(t, alice.Description, "description Алисы не должен быть nil")
	assert.Equal(t, "Основной счёт", *alice.Description)

	// Боб НЕ имеет description (NULL)
	bob, err := repo.Get(ctx, bobUUID)
	require.NoError(t, err)
	assert.Nil(t, bob.Description, "description Боба должен быть nil")
}
