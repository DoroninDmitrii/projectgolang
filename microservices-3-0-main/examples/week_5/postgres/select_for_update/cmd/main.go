package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	accountRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/repository/account"
	accountService "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/service/account"
)

const (
	aliceUUID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001"
	bobUUID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002"

	aliceInitBalance = 1_000_000 // начальный баланс Алисы: 10 000₽ (в копейках)
	bobInitBalance   = 500_000   // начальный баланс Боба: 5 000₽ (в копейках)

	transferAmount    = 1000 // 10₽ в копейках
	concurrentWorkers = 20   // количество параллельных горутин
)

func main() {
	if err := run(); err != nil {
		slog.Error("ошибка выполнения", "error", err)
		os.Exit(1)
	}
}

//nolint:funlen // учебный пример с демонстрацией двух сценариев переводов
func run() error {
	ctx := context.Background()

	err := godotenv.Load(".env")
	if err != nil {
		return err
	}

	dbURI := os.Getenv("DB_URI")

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err = pool.Ping(ctx); err != nil {
		return err
	}

	slog.Info("подключение к PostgreSQL установлено")

	txManager, err := manager.New(trmpgx.NewDefaultFactory(pool))
	if err != nil {
		return err
	}

	repo := accountRepo.New(pool)
	svc := accountService.New(txManager, repo)

	totalTransfer := int64(concurrentWorkers) * transferAmount
	expectedAlice := int64(aliceInitBalance) - totalTransfer
	expectedBob := int64(bobInitBalance) + totalTransfer

	slog.Info(
		"Параметры демо",
		"перевод", fmtRub(transferAmount),
		"направление", "Алиса → Боб",
		"горутин", concurrentWorkers,
		"итого_переводов", fmtRub(totalTransfer),
	)

	// === Демо 1: Небезопасные параллельные переводы ===
	slog.Info("═══ Демо 1: Без FOR UPDATE (небезопасно) ═══")

	if err = resetBalances(ctx, pool); err != nil {
		return err
	}

	printBalances(ctx, pool, "Начальные балансы")

	var errCount atomic.Int64

	var wg sync.WaitGroup
	for range concurrentWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if transferErr := svc.TransferUnsafe(ctx, aliceUUID, bobUUID, transferAmount); transferErr != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	alice, bob := getBalances(ctx, pool)
	slog.Info(
		"Результат без FOR UPDATE",
		"alice_факт", fmtRub(alice),
		"alice_ожидание", fmtRub(expectedAlice),
		"bob_факт", fmtRub(bob),
		"bob_ожидание", fmtRub(expectedBob),
		"total", fmtRub(alice+bob),
		"ошибок", errCount.Load(),
	)
	if alice != expectedAlice || bob != expectedBob {
		slog.Warn("Lost update! Балансы не совпадают с ожидаемыми — часть переводов потерялась")
	}

	// === Демо 2: Безопасные параллельные переводы ===
	slog.Info("═══ Демо 2: С FOR UPDATE (безопасно) ═══")

	if err = resetBalances(ctx, pool); err != nil {
		return err
	}

	printBalances(ctx, pool, "Начальные балансы")

	errCount.Store(0)

	for range concurrentWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if transferErr := svc.Transfer(ctx, aliceUUID, bobUUID, transferAmount); transferErr != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	alice, bob = getBalances(ctx, pool)
	slog.Info(
		"Результат с FOR UPDATE",
		"alice_факт", fmtRub(alice),
		"alice_ожидание", fmtRub(expectedAlice),
		"bob_факт", fmtRub(bob),
		"bob_ожидание", fmtRub(expectedBob),
		"total", fmtRub(alice+bob),
		"ошибок", errCount.Load(),
	)
	if alice == expectedAlice && bob == expectedBob {
		slog.Info("FOR UPDATE предотвратил lost update — все переводы учтены корректно")
	}

	return nil
}

func resetBalances(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `UPDATE accounts SET balance = $2, updated_at = NULL WHERE uuid = $1`, aliceUUID, aliceInitBalance)
	if err != nil {
		return fmt.Errorf("сбросить баланс Алисы: %w", err)
	}

	_, err = pool.Exec(ctx, `UPDATE accounts SET balance = $2, updated_at = NULL WHERE uuid = $1`, bobUUID, bobInitBalance)
	if err != nil {
		return fmt.Errorf("сбросить баланс Боба: %w", err)
	}

	return nil
}

func getBalances(ctx context.Context, pool *pgxpool.Pool) (alice, bob int64) {
	err := pool.QueryRow(ctx, "SELECT balance FROM accounts WHERE uuid = $1", aliceUUID).Scan(&alice)
	if err != nil {
		slog.Error("не удалось получить баланс Алисы", "error", err)
	}

	err = pool.QueryRow(ctx, "SELECT balance FROM accounts WHERE uuid = $1", bobUUID).Scan(&bob)
	if err != nil {
		slog.Error("не удалось получить баланс Боба", "error", err)
	}

	return alice, bob
}

func printBalances(ctx context.Context, pool *pgxpool.Pool, label string) {
	alice, bob := getBalances(ctx, pool)
	slog.Info(
		label,
		"alice", fmtRub(alice),
		"bob", fmtRub(bob),
		"total", fmtRub(alice+bob),
	)
}

// fmtRub форматирует копейки в рубли с разделением разрядов пробелами
// Например: 1000000 → "10 000.00 ₽", 1000 → "10.00 ₽".
func fmtRub(kopecks int64) string {
	rubles := kopecks / 100
	kop := kopecks % 100

	// Форматируем рубли с разделителями разрядов (пробел)
	s := strconv.FormatInt(rubles, 10)
	n := len(s)
	if n <= 3 {
		return fmt.Sprintf("%s.%02d ₽", s, kop)
	}

	var result []byte
	for i, ch := range s {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ' ')
		}
		result = append(result, byte(ch)) //nolint:gosec // G115: ch — цифра ASCII, overflow невозможен
	}

	return fmt.Sprintf("%s.%02d ₽", string(result), kop)
}
