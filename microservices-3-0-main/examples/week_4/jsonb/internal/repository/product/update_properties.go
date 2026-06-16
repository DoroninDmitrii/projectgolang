package product

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
)

// UpdateProperties полностью заменяет JSONB-поле properties
// Новые properties сериализуются через json.Marshal и передаются как []byte
func (r *repository) UpdateProperties(ctx context.Context, id string, props model.ProductProperties) error {
	const query = `UPDATE products SET properties = $1, updated_at = $2 WHERE id = $3`

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return fmt.Errorf("сериализовать properties: %w", err)
	}

	now := time.Now()

	tag, err := r.pool.Exec(ctx, query, propsJSON, now, id)
	if err != nil {
		return fmt.Errorf("обновить properties: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrProductNotFound
	}

	return nil
}
