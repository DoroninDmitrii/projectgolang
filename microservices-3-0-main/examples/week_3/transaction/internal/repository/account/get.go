package account

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/record"
)

func (r *repository) Get(ctx context.Context, uuid string) (model.Account, error) {
	const query = `SELECT uuid, owner, balance, description, created_at, updated_at
		FROM accounts WHERE uuid = $1`

	var rec record.Account

	// DefaultTrOrDB: если ctx содержит транзакцию (вызов пришёл из txManager.Do) —
	// запрос выполнится внутри этой транзакции. Иначе — через обычный пул
	err := r.getter.DefaultTrOrDB(ctx, r.pool).QueryRow(ctx, query, uuid).Scan(
		&rec.UUID, &rec.Owner, &rec.Balance, &rec.Description, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Account{}, errs.ErrAccountNotFound
		}

		return model.Account{}, err
	}

	// Scan в record-структуру, на выход — доменная модель через конвертер
	return repoConverter.AccountToModel(rec), nil
}
