package product

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// List возвращает все товары
func (s *Service) List(ctx context.Context) ([]model.Product, error) {
	products, err := s.productRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("получить список товаров: %w", err)
	}

	return products, nil
}
