package account

import (
	trmpgx "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool   *pgxpool.Pool
	getter *trmpgx.CtxGetter
}

// New создаёт репозиторий счетов, работающий с PostgreSQL
func New(pool *pgxpool.Pool) *repository {
	return &repository{
		pool:   pool,
		getter: trmpgx.DefaultCtxGetter,
	}
}
