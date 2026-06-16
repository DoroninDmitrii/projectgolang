package ufo

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт новый экземпляр репозитория
func NewRepository(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}
