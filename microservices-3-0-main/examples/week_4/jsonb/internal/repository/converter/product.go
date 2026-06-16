// Package converter содержит конвертеры между доменными моделями и моделями хранилища
package converter

import (
	"encoding/json"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/record"
)

// ProductInfoToRecord собирает запись для INSERT из id + ProductInfo.
// JSON-сериализация доменных Properties в сырые байты JSONB — здесь, чтобы
// репозиторий работал только с record и не лез в json.Marshal сам.
func ProductInfoToRecord(id string, info model.ProductInfo) (record.Product, error) {
	propsJSON, err := json.Marshal(info.Properties)
	if err != nil {
		return record.Product{}, fmt.Errorf("сериализовать properties: %w", err)
	}

	return record.Product{
		ID:          id,
		Name:        info.Name,
		ProductType: string(info.ProductType),
		Properties:  propsJSON,
	}, nil
}

// ProductToModel преобразует запись таблицы products в доменную модель.
// JSONB-колонка десериализуется в model.ProductProperties.
func ProductToModel(rec record.Product) (model.Product, error) {
	var props model.ProductProperties
	if err := json.Unmarshal(rec.Properties, &props); err != nil {
		return model.Product{}, fmt.Errorf("десериализовать properties: %w", err)
	}

	return model.Product{
		ID:          rec.ID,
		Name:        rec.Name,
		ProductType: model.ProductType(rec.ProductType),
		Properties:  props,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}, nil
}
