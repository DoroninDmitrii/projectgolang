package account

import (
	"context"
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/errors"
)

func (r *repository) UpdateBalance(ctx context.Context, uuid string, newBalance int64) error {
	const query = `UPDATE accounts SET balance = $1, updated_at = $2 WHERE uuid = $3`

	now := time.Now()

	tag, err := r.getter.DefaultTrOrDB(ctx, r.pool).Exec(ctx, query, newBalance, now, uuid)
	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrAccountNotFound
	}

	return nil
}
