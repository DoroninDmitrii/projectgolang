package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/record"
)

// Get получает товар по ID. JSONB-колонка читается в record как сырые []byte,
// json.Unmarshal в доменную ProductProperties делает конвертер.
func (r *repository) Get(ctx context.Context, id string) (model.Product, error) {
	const query = `SELECT id, name, product_type, properties, created_at, updated_at
		FROM products WHERE id = $1`

	var rec record.Product

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&rec.ID, &rec.Name, &rec.ProductType, &rec.Properties, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Product{}, errs.ErrProductNotFound
		}

		return model.Product{}, fmt.Errorf("получить товар: %w", err)
	}

	return repoConverter.ProductToModel(rec)
}
