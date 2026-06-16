package account

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/model"
	repoConverter "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/converter"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/repository/record"
)

func (r *repository) List(ctx context.Context, uuids []string) ([]model.Account, error) {
	const query = `SELECT uuid, owner, balance, description, created_at, updated_at
		FROM accounts WHERE uuid = ANY($1)`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, uuids)
	if err != nil {
		return nil, fmt.Errorf("получить список счетов: %w", err)
	}
	defer rows.Close()

	// CollectRows собирает строки в record-структуры — позиционный маппинг по
	// порядку SELECT'а. Конверсия в доменные model.Account — следующим шагом.
	records, err := pgx.CollectRows(rows, pgx.RowToStructByPos[record.Account])
	if err != nil {
		return nil, fmt.Errorf("считать строки: %w", err)
	}

	accounts := make([]model.Account, 0, len(records))
	for _, rec := range records {
		accounts = append(accounts, repoConverter.AccountToModel(rec))
	}

	return accounts, nil
}
