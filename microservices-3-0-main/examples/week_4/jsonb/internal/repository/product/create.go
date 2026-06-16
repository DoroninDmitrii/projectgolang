package product

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/converter"
)

// Create вставляет новый товар с заранее сгенерированным id (UUID создаётся на сервисном слое).
// Доменный ProductInfo прогоняется через конвертер: он же делает json.Marshal
// Properties — репозиторий работает уже с record.Product.
func (r *repository) Create(ctx context.Context, id string, info model.ProductInfo) error {
	rec, err := repoConverter.ProductInfoToRecord(id, info)
	if err != nil {
		return err
	}

	const query = `INSERT INTO products (id, name, product_type, properties)
		VALUES ($1, $2, $3, $4)`

	if _, err := r.pool.Exec(ctx, query, rec.ID, rec.Name, rec.ProductType, rec.Properties); err != nil {
		return fmt.Errorf("вставить товар: %w", err)
	}

	return nil
}
