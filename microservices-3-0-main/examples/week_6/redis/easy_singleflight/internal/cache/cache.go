package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// Cache предоставляет кеширование товаров в Redis Hash с защитой от cache stampede
type Cache struct {
	rdb *redis.Client
	sf  singleflight.Group
	ttl time.Duration
}

// New создаёт кеш с заданным Redis-клиентом и TTL
func New(rdb *redis.Client, ttl time.Duration) *Cache {
	return &Cache{
		rdb: rdb,
		ttl: ttl,
	}
}

// Set сохраняет товар в Redis Hash с TTL
func (c *Cache) Set(ctx context.Context, product *Product) error {
	key := "product:" + product.ID

	// Pipeline буферизует несколько команд и отправляет их в Redis за один round-trip
	// Без pipeline каждая команда — отдельный сетевой запрос-ответ:
	//   HSET → OK, EXPIRE → OK  (2 round-trip'а)
	// С pipeline обе команды уходят одним TCP-пакетом, ответы приходят разом:
	//   HSET + EXPIRE → OK, OK  (1 round-trip)
	//
	// Важно: pipeline НЕ атомарен — команды выполняются последовательно на сервере,
	// но между ними могут вклиниться команды от других клиентов, а при обрыве соединения
	// HSET может выполниться, а EXPIRE — нет (ключ останется без TTL → утечка памяти)
	//
	// Если нужна атомарность, есть два варианта:
	//   1. MULTI/EXEC (транзакция) — гарантирует, что все команды выполнятся как единое целое
	//   2. Lua-скрипт — атомарное выполнение на сервере, тоже за один round-trip
	//
	// Для кеша pipeline — прагматичный и достаточный выбор: в худшем случае
	// при сбое ключ просто не получит TTL, а при следующей записи всё исправится
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, key, product)
	pipe.Expire(ctx, key, c.ttl)

	_, err := pipe.Exec(ctx)

	return err
}

// Del удаляет товар из кеша
func (c *Cache) Del(ctx context.Context, productID string) error {
	return c.rdb.Del(ctx, "product:"+productID).Err()
}

// Get получает товар из Redis Hash
// Возвращает ErrCacheMiss, если ключ не найден
func (c *Cache) Get(ctx context.Context, productID string) (*Product, error) {
	key := "product:" + productID

	cmd := c.rdb.HGetAll(ctx, key)
	if err := cmd.Err(); err != nil {
		return nil, err
	}

	if len(cmd.Val()) == 0 {
		return nil, ErrCacheMiss
	}

	var product Product
	if err := cmd.Scan(&product); err != nil {
		return nil, err
	}

	return &product, nil
}
