package cache

import (
	"context"
	"errors"
	"log/slog"
)

// GetProduct получает товар: сначала из кеша, при промахе — из БД
// Без защиты от cache stampede — каждая горутина самостоятельно лезет в БД
func (c *Cache) GetProduct(ctx context.Context, productID string, fetchFn FetchFunc) (*Product, error) {
	// Пробуем кеш
	product, err := c.Get(ctx, productID)
	if nil == err {
		return product, nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return nil, err
	}

	// Cache miss — идём в БД. Каждая горутина делает это независимо!
	product, err = fetchFn(ctx, productID)
	if err != nil {
		return nil, err
	}

	// Записываем в кеш
	if cacheErr := c.Set(ctx, product); cacheErr != nil {
		slog.Warn("не удалось записать в кеш", "error", cacheErr)
	}

	return product, nil
}
