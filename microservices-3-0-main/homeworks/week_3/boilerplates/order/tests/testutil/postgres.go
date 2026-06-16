// Package testutil содержит инфраструктуру для E2E-тестов:
// единый PostgreSQL-контейнер на весь пакет, schema-per-test изоляцию,
// поднятие гRPC-сервисов через bufconn и HTTP через httptest, плюс
// helpers для seed-данных и прямой проверки состояния БД.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// pgContainer — единый PostgreSQL-контейнер на весь пакет.
// Создаётся лениво при первом вызове sharedContainer().
var (
	pgOnce       sync.Once
	pgContainer  *tcpostgres.PostgresContainer
	pgBaseDSN    string
	pgInitErr    error
	pgDBCounter  uint64
	pgPostgresDB = "postgres" // системная БД для CREATE DATABASE
)

// dbInfo описывает созданную для теста БД.
type dbInfo struct {
	Name    string
	DSN     string
	cleanup func()
}

// sharedContainer гарантирует, что один и тот же PostgreSQL-контейнер
// используется всеми тестами пакета. Контейнер останавливается через
// StopShared в TestMain.
func sharedContainer(ctx context.Context) (*tcpostgres.PostgresContainer, string, error) {
	pgOnce.Do(func() {
		c, err := tcpostgres.Run(
			ctx,
			"postgres:18.3-alpine3.23",
			tcpostgres.WithDatabase(pgPostgresDB),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if err != nil {
			pgInitErr = err
			return
		}

		dsn, err := c.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgInitErr = err
			return
		}

		pgContainer = c
		pgBaseDSN = dsn
	})

	return pgContainer, pgBaseDSN, pgInitErr
}

// StopShared останавливает контейнер. Вызывается из TestMain после всех тестов.
func StopShared(ctx context.Context) error {
	if pgContainer == nil {
		return nil
	}
	return pgContainer.Terminate(ctx)
}

// createIsolatedDB создаёт уникальную БД, накатывает миграции из migrationsDir
// и возвращает DSN. cleanup удаляет БД.
func createIsolatedDB(ctx context.Context, t *testing.T, prefix, migrationsDir string) dbInfo {
	t.Helper()

	_, baseDSN, err := sharedContainer(ctx)
	if err != nil {
		t.Fatalf("postgres контейнер: %v", err)
	}

	id := atomic.AddUint64(&pgDBCounter, 1)
	dbName := fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), id)

	// Подключаемся к системной БД и создаём новую.
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("открыть admin connection: %v", err)
	}
	defer func() { _ = adminDB.Close() }()

	if _, err = adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName)); err != nil {
		t.Fatalf("создать БД %s: %v", dbName, err)
	}

	// DSN на новую БД получаем заменой имени системной БД на dbName.
	// pgContainer.ConnectionString не позволяет указать произвольную БД, поэтому
	// строим DSN на основе baseDSN.
	dsn := replaceDBName(baseDSN, pgPostgresDB, dbName)

	// Накатываем миграции в новую БД.
	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("открыть migrations connection: %v", err)
	}
	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		_ = migrateDB.Close()
		t.Fatalf("filepath.Abs: %v", err)
	}
	if err = goose.Up(migrateDB, absDir); err != nil {
		_ = migrateDB.Close()
		t.Fatalf("goose up: %v", err)
	}
	_ = migrateDB.Close()

	cleanup := func() {
		// Подключаемся к admin БД, чтобы дропнуть рабочую.
		admin, err := sql.Open("pgx", baseDSN)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close() }()

		// Завершаем активные коннекты и удаляем БД.
		_, _ = admin.Exec(fmt.Sprintf(
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			 WHERE datname = '%s' AND pid <> pg_backend_pid()`, dbName,
		))
		_, _ = admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, dbName))
	}

	return dbInfo{Name: dbName, DSN: dsn, cleanup: cleanup}
}

// replaceDBName меняет имя БД в DSN. testcontainers даёт DSN вида
// "postgres://user:pass@host:port/postgres?sslmode=disable" — заменяем
// последний сегмент пути.
func replaceDBName(dsn, oldDB, newDB string) string {
	// dsn: postgres://user:pass@host:port/oldDB?...
	// ищем "/oldDB" перед "?".
	old := "/" + oldDB
	idx := -1
	for i := len(dsn) - 1; i >= 0; i-- {
		if i+len(old) <= len(dsn) && dsn[i:i+len(old)] == old {
			// убедимся, что следующий символ либо "?", либо конец строки.
			next := i + len(old)
			if next == len(dsn) || dsn[next] == '?' {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		// fallback — возвращаем dsn как есть, тест упадёт явно.
		return dsn
	}
	return dsn[:idx] + "/" + newDB + dsn[idx+len(old):]
}
