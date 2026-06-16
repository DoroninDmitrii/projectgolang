package product

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/record"
)

// List возвращает все товары. Каждая строка читается в record (Properties — сырые
// JSONB-байты), а доменная Product собирается через конвертер — он же десериализует JSON.
func (r *repository) List(ctx context.Context) ([]model.Product, error) {
	const query = `SELECT id, name, product_type, properties, created_at, updated_at
		FROM products ORDER BY created_at`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("получить список товаров: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var rec record.Product

		if err = rows.Scan(
			&rec.ID, &rec.Name, &rec.ProductType, &rec.Properties, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("считать строку: %w", err)
		}

		p, err := repoConverter.ProductToModel(rec)
		if err != nil {
			return nil, err
		}

		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("итерация строк: %w", err)
	}

	return products, nil
}
