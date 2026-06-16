package product

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// UpdateProperties обновляет properties товара
func (s *Service) UpdateProperties(ctx context.Context, id string, props model.ProductProperties) error {
	if err := s.productRepo.UpdateProperties(ctx, id, props); err != nil {
		return fmt.Errorf("обновить properties товара: %w", err)
	}

	return nil
}
