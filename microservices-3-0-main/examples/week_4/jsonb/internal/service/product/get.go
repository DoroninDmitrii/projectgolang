package product

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// Get возвращает товар по ID
func (s *Service) Get(ctx context.Context, id string) (model.Product, error) {
	product, err := s.productRepo.Get(ctx, id)
	if err != nil {
		return model.Product{}, fmt.Errorf("получить товар: %w", err)
	}

	return product, nil
}
