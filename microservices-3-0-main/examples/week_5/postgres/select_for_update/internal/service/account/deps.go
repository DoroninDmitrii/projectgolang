package account

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/model"
)

// TxManager определяет контракт для управления транзакциями
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// AccountRepository определяет контракт для работы с хранилищем счетов
type AccountRepository interface {
	Get(ctx context.Context, uuid string) (model.Account, error)
	GetForUpdate(ctx context.Context, uuid string) (model.Account, error)
	List(ctx context.Context, uuids []string) ([]model.Account, error)
	Create(ctx context.Context, account model.Account) error
	UpdateBalance(ctx context.Context, uuid string, newBalance int64) error
}
