package weather

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_3/workspace/http/internal/model"
)

// Upsert вставляет новую запись или обновляет существующую (INSERT ... ON CONFLICT ... DO UPDATE)
//
// Это паттерн "UPSERT" (UPdate + inSERT) — атомарная операция в PostgreSQL,
// которая за один запрос решает: вставить новую строку или обновить существующую
//
// Как работает:
//  1. PostgreSQL пытается выполнить INSERT
//  2. Если возникает конфликт по уникальному ключу (ON CONFLICT (city)),
//     вместо ошибки выполняется DO UPDATE SET — обновление существующей строки
//  3. RETURNING возвращает итоговую строку (неважно, была вставка или обновление)
//
// Плюсы:
//   - Атомарность: нет race condition между SELECT + INSERT/UPDATE
//     Два одновременных запроса для одного city не создадут дубликат
//   - Один запрос вместо двух-трёх (SELECT → if exists → UPDATE else INSERT)
//   - PostgreSQL сам решает, вставлять или обновлять — нет логики на стороне приложения
//
// Минусы:
//   - Требует уникальный индекс/constraint на колонку конфликта (city)
//   - При DO UPDATE всегда перезаписывает указанные колонки целиком
//   - Инкрементирует sequence (если есть serial/identity PK), даже при UPDATE-ветке
//
// Когда использовать:
//   - UPSERT (ON CONFLICT DO UPDATE) — когда семантика UPDATE: клиент передаёт ВСЕ поля,
//     и мы перезаписываем строку целиком. Пример: «установить температуру в Москве = 25°»
//     Здесь нет понятия «оставить старое значение» — клиент всегда знает полное состояние
//   - COALESCE — когда семантика PATCH: клиент передаёт только ИЗМЕНЁННЫЕ поля,
//     а остальные должны сохранить текущее значение. Пример: «обновить только цвет НЛО,
//     не трогая остальные поля». См. repository/ufo/update.go в grpc-модуле
func (r *repository) Upsert(ctx context.Context, city string, temperature float64) (model.Weather, error) {
	query := `INSERT INTO weather (city, temperature, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (city) DO UPDATE SET temperature = $2, updated_at = now()
		RETURNING city, temperature, updated_at`

	var w model.Weather
	err := r.pool.QueryRow(ctx, query, city, temperature).Scan(&w.City, &w.Temperature, &w.UpdatedAt)
	if err != nil {
		return model.Weather{}, err
	}

	return w, nil
}
