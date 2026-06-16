package cache

import "context"

// Product — товар в интернет-магазине
type Product struct {
	ID    string `redis:"id"`
	Name  string `redis:"name"`
	Price int    `redis:"price"`
}

// FetchFunc — функция получения товара из БД
// Позволяет подставить реальную или тестовую реализацию
type FetchFunc func(ctx context.Context, productID string) (*Product, error)
