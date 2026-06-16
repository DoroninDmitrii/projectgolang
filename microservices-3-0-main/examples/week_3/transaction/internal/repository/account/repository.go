package account

import (
	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool *pgxpool.Pool

	// getter извлекает активную транзакцию (pgx.Tx) из context.Context
	// Подробнее о том, как работает CtxGetter и зачем нужен кастомный — см. README.md
	getter *trmpgx.CtxGetter
}

// New создаёт репозиторий счетов, работающий с PostgreSQL
func New(pool *pgxpool.Pool) *repository {
	return &repository{
		pool: pool,
		// DefaultCtxGetter — готовый экземпляр из go-transaction-manager,
		// который знает, как достать pgx.Tx из context.Context
		getter: trmpgx.DefaultCtxGetter,
	}
}
