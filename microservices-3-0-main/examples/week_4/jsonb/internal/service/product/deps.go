package product

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// ProductRepository определяет контракт для работы с хранилищем товаров
type ProductRepository interface {
	Create(ctx context.Context, id string, info model.ProductInfo) error
	Get(ctx context.Context, id string) (model.Product, error)
	List(ctx context.Context) ([]model.Product, error)
	UpdateProperties(ctx context.Context, id string, props model.ProductProperties) error
}
