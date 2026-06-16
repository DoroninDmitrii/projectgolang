// Package converter содержит конвертеры между доменными моделями и моделями хранилища
package converter

import (
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/model"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/record"
)

// AccountToModel преобразует запись из таблицы accounts в доменную модель.
// На этой неделе обратного конвертера (AccountToRecord) нет — Create в репо
// не реализован, а UpdateBalance принимает скаляры, не целый объект.
func AccountToModel(rec record.Account) model.Account {
	return model.Account{
		UUID:        rec.UUID,
		Owner:       rec.Owner,
		Balance:     rec.Balance,
		Description: rec.Description,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}
}
