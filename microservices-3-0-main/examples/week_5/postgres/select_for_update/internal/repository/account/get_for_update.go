package account

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/model"
)

// GetForUpdate получает счёт по UUID с блокировкой строки (SELECT ... FOR UPDATE)
// Блокировка удерживается до конца транзакции и не позволяет другим транзакциям
// читать или изменять эту строку, предотвращая race condition
func (r *repository) GetForUpdate(ctx context.Context, uuid string) (model.Account, error) {
	const query = `SELECT uuid, owner, balance, description, created_at, updated_at
		FROM accounts WHERE uuid = $1 FOR UPDATE`

	var a model.Account
	err := r.getter.DefaultTrOrDB(ctx, r.pool).QueryRow(ctx, query, uuid).Scan(
		&a.UUID, &a.Owner, &a.Balance, &a.Description, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Account{}, errs.ErrAccountNotFound
		}

		return model.Account{}, err
	}

	return a, nil
}
