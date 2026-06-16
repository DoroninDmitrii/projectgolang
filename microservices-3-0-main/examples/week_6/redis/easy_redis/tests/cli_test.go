package tests

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"testing"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

var redisAddr string

// TestMain поднимает Redis через testcontainers и запускает тесты
func TestMain(m *testing.M) {
	_ = godotenv.Load("../.env")

	ctx := context.Background()

	// Поднимаем Redis в контейнере на случайном порту
	redisContainer, err := tcredis.Run(ctx, os.Getenv("REDIS_IMAGE"))
	if err != nil {
		slog.Error("не удалось запустить Redis контейнер", "error", err)
		os.Exit(1)
	}

	// Получаем хост и порт контейнера
	host, err := redisContainer.Host(ctx)
	if err != nil {
		slog.Error("не удалось получить хост контейнера", "error", err)
		os.Exit(1)
	}

	port, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		slog.Error("не удалось получить порт контейнера", "error", err)
		os.Exit(1)
	}

	redisAddr = fmt.Sprintf("%s:%s", host, port.Port())

	code := m.Run()

	if termErr := redisContainer.Terminate(ctx); termErr != nil {
		slog.Error("не удалось остановить контейнер", "error", termErr)
	}

	os.Exit(code)
}

// runProgram запускает Go программу из каталога проекта и возвращает вывод
// REDIS_HOST и REDIS_PORT передаются через переменные окружения (testcontainers назначает случайный порт)
func runProgram(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("go", "run", "./cmd/main.go")
	cmd.Dir = ".." // запуск из easy_redis/

	// Парсим хост и порт из redisAddr
	host, port, _ := parseAddr(redisAddr)
	cmd.Env = append(
		os.Environ(),
		"REDIS_HOST="+host,
		"REDIS_PORT="+port,
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "программа завершилась с ошибкой:\n%s", string(output))

	return string(output)
}

// parseAddr разбирает адрес "host:port" на компоненты
func parseAddr(addr string) (string, string, error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}

	return addr, "", fmt.Errorf("нет порта в адресе: %s", addr)
}

// newRedisClient создаёт клиент Redis для тестов
func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
}

// flushAll очищает все данные в Redis
func flushAll(t *testing.T) {
	t.Helper()

	rdb := newRedisClient()
	defer rdb.Close()

	err := rdb.FlushAll(context.Background()).Err()
	require.NoError(t, err, "не удалось выполнить FLUSHALL")
}

// dbSize возвращает количество ключей в Redis
func dbSize(t *testing.T) int {
	t.Helper()

	rdb := newRedisClient()
	defer rdb.Close()

	size, err := rdb.DBSize(context.Background()).Result()
	require.NoError(t, err, "не удалось получить DBSIZE")

	return int(size)
}

// TestEasyRedis_Success проверяет успешное выполнение программы
func TestEasyRedis_Success(t *testing.T) {
	flushAll(t)

	output := runProgram(t)

	assert.Contains(t, output, "пара ключ-значение", "вывод должен содержать результат SET/GET")
	assert.Contains(t, output, "данные пользователя", "вывод должен содержать данные из HSET/HGET")
}

// TestEasyRedis_WritesKeys проверяет, что программа записывает ключи в Redis
func TestEasyRedis_WritesKeys(t *testing.T) {
	flushAll(t)

	runProgram(t)

	// Программа создаёт 1 string-ключ (SET) и 1 hash-ключ (HSET) = 2 ключа
	assert.Equal(t, 2, dbSize(t), "программа должна создать 2 ключа в Redis")
}

// TestEasyRedis_Idempotent проверяет, что программу можно запустить несколько раз подряд
func TestEasyRedis_Idempotent(t *testing.T) {
	flushAll(t)

	for i := range 3 {
		output := runProgram(t)
		assert.Contains(t, output, "пара ключ-значение",
			"запуск %d должен содержать результат SET/GET", i+1)

		// Каждый запуск добавляет 2 новых ключа (UUID уникальный)
		assert.Equal(t, (i+1)*2, dbSize(t),
			"после запуска %d должно быть %d ключей", i+1, (i+1)*2)
	}
}
