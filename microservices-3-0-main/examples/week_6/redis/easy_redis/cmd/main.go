package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

const (
	fieldName     = "name"
	fieldLastName = "last_name"
	fieldAge      = "age"
	fieldEmail    = "email"
)

type User struct {
	Name     string `redis:"name"`
	LastName string `redis:"last_name"`
	Age      int    `redis:"age"`
	Email    string `redis:"email"`
}

func main() {
	// Загружаем переменные окружения из .env (если файл существует)
	_ = godotenv.Load() //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	// Подключаемся к Redis
	redisHost := cmp.Or(os.Getenv("REDIS_HOST"), "localhost")
	redisPort := cmp.Or(os.Getenv("REDIS_PORT"), "6379")

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
	})
	defer func() {
		if cerr := rdb.Close(); cerr != nil {
			slog.Error("не удалось закрыть соединение с Redis", "error", cerr)
		}
	}()

	ctx := context.Background()

	setAndGet(ctx, rdb)
	hsetAndHGet(ctx, rdb)
}

func setAndGet(ctx context.Context, rdb *redis.Client) {
	key := gofakeit.UUID()
	value := gofakeit.FirstName()

	// Сохраняем пару ключ-значение
	err := rdb.Set(ctx, key, value, 0).Err()
	if err != nil {
		slog.Error("не удалось установить ключ", "error", err)
		return
	}

	// Получаем значение по ключу
	value, err = rdb.Get(ctx, key).Result()
	if err != nil {
		slog.Error("не удалось получить ключ", "error", err)
		return
	}

	slog.Info("пара ключ-значение", "key", key, "value", value)
}

func hsetAndHGet(ctx context.Context, rdb *redis.Client) {
	hashKey := gofakeit.UUID()
	user := User{
		Name:     gofakeit.FirstName(),
		LastName: gofakeit.LastName(),
		Age:      gofakeit.IntRange(0, 100),
		Email:    gofakeit.Email(),
	}

	// Сохраняем структуру в хеш-таблицу
	// HSet принимает структуру с тегами `redis:"..."` и сам конвертирует поля,
	// включая числовые — не нужно вручную вызывать strconv
	err := rdb.HSet(ctx, hashKey, user).Err()
	if err != nil {
		slog.Error("не удалось установить поля хеша", "error", err)
		return
	}

	// Получаем значения из хеш-таблицы разными способами
	printMapFieldsByOne(ctx, rdb, hashKey)
	printMapFields(ctx, rdb, hashKey)
	printMapFieldsByStruct(ctx, rdb, hashKey)
}

func printMapFieldsByOne(ctx context.Context, rdb *redis.Client, hashKey string) {
	name, err := rdb.HGet(ctx, hashKey, fieldName).Result()
	if err != nil {
		slog.Error("не удалось получить поле хеша", "field", fieldName, "error", err)
		return
	}

	lastName, err := rdb.HGet(ctx, hashKey, fieldLastName).Result()
	if err != nil {
		slog.Error("не удалось получить поле хеша", "field", fieldLastName, "error", err)
		return
	}

	age, err := rdb.HGet(ctx, hashKey, fieldAge).Result()
	if err != nil {
		slog.Error("не удалось получить поле хеша", "field", fieldAge, "error", err)
		return
	}

	email, err := rdb.HGet(ctx, hashKey, fieldEmail).Result()
	if err != nil {
		slog.Error("не удалось получить поле хеша", "field", fieldEmail, "error", err)
		return
	}

	slog.Info("данные пользователя с идентифкатором", "hash_key", hashKey)
	slog.Info("данные пользователя", "name", name, "last_name", lastName, "age", age, "email", email)
}

func printMapFields(ctx context.Context, rdb *redis.Client, hashKey string) {
	hashMap, err := rdb.HGetAll(ctx, hashKey).Result()
	if err != nil {
		slog.Error("не удалось получить все поля хеша", "error", err)
		return
	}

	slog.Info("данные пользователя с идентифкатором (полученные разом)", "hash_key", hashKey, "data", hashMap)
}

func printMapFieldsByStruct(ctx context.Context, rdb *redis.Client, hashKey string) {
	// HGetAll().Scan() сканирует поля хеша в структуру по тегам `redis:"..."`,
	// автоматически конвертируя строки Redis в нужные Go-типы (int, string и т.д.)
	var user User
	err := rdb.HGetAll(ctx, hashKey).Scan(&user)
	if err != nil {
		slog.Error("не удалось отсканировать поля хеша в структуру", "error", err)
		return
	}

	slog.Info("данные пользователя с идентифкатором (распаршенные в структуру)", "hash_key", hashKey, "user", user)
}
