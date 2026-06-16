package account

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/model"
)

func (r *repository) List(ctx context.Context, uuids []string) ([]model.Account, error) {
	const query = `SELECT uuid, owner, balance, description, created_at, updated_at
		FROM accounts WHERE uuid = ANY($1)`

	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx, query, uuids)
	if err != nil {
		return nil, fmt.Errorf("получить список счетов: %w", err)
	}
	defer rows.Close()

	// pgx.CollectRows собирает все строки результата в слайс, применяя к каждой строке
	// функцию-маппер. pgx.RowToStructByName автоматически сканирует колонки в поля структуры,
	// сопоставляя имена колонок с тегами `db:"..."` на полях структуры
	// Это удобнее ручного Scan, особенно когда колонок много
	accounts, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Account])
	if err != nil {
		return nil, fmt.Errorf("считать строки: %w", err)
	}

	return accounts, nil
}
