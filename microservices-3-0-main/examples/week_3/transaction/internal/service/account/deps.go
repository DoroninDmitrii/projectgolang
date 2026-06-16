package account

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/model"
)

// TxManager определяет контракт для управления транзакциями
// Реализация — *manager.Manager из библиотеки go-transaction-manager
//
// Метод Do оборачивает callback в SQL-транзакцию (BEGIN/COMMIT/ROLLBACK):
//   - Перед вызовом fn: выполняет BEGIN и кладёт pgx.Tx внутрь нового ctx
//   - fn возвращает nil → выполняет COMMIT
//   - fn возвращает error → выполняет ROLLBACK
//
// Ключевой момент: ctx, который получает fn, содержит внутри себя активную
// транзакцию. Именно этот ctx нужно передавать дальше в репозиторий — тогда
// все запросы пойдут через одно соединение в рамках одной транзакции
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// AccountRepository определяет контракт для работы с хранилищем счетов
type AccountRepository interface {
	Get(ctx context.Context, uuid string) (model.Account, error)
	List(ctx context.Context, uuids []string) ([]model.Account, error)
	UpdateBalance(ctx context.Context, uuid string, newBalance int64) error
}
