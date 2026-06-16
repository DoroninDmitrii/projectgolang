package product

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// Create создаёт новый товар. UUID генерируется здесь, на сервисном слое,
// и передаётся в репозиторий как явный параметр запроса.
func (s *Service) Create(ctx context.Context, info model.ProductInfo) (string, error) {
	id := uuid.NewString()

	if err := s.productRepo.Create(ctx, id, info); err != nil {
		return "", fmt.Errorf("создать товар: %w", err)
	}

	return id, nil
}
