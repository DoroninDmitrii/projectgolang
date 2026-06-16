package account

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/model"
)

func (r *repository) Create(ctx context.Context, account model.Account) error {
	const query = `INSERT INTO accounts (uuid, owner, balance, description, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.getter.DefaultTrOrDB(ctx, r.pool).Exec(
		ctx, query,
		account.UUID, account.Owner, account.Balance, account.Description, account.CreatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}
