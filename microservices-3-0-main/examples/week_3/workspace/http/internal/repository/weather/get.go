package weather

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/model"
)

func (r *repository) Get(ctx context.Context, city string) (model.Weather, error) {
	query := `SELECT city, temperature, updated_at FROM weather WHERE city = $1`

	var w model.Weather
	err := r.pool.QueryRow(ctx, query, city).Scan(&w.City, &w.Temperature, &w.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Weather{}, errs.ErrWeatherNotFound
		}
		return model.Weather{}, err
	}

	return w, nil
}
