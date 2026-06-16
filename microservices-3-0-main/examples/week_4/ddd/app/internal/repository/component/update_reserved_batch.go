package component

import (
	"context"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
)

// UpdateReservedBatch обновляет количество зарезервированных единиц для нескольких компонентов одним запросом
//
// Используется функция PostgreSQL unnest — она «разворачивает» два массива (UUID и reserved)
// в виртуальную таблицу batch с колонками (uuid, reserved). Каждая пара элементов
// массивов на одной позиции становится одной строкой этой таблицы
//
// Пример: unnest(ARRAY['aaa','bbb'], ARRAY[3,5]) → строки ('aaa',3), ('bbb',5)
//
// Затем UPDATE ... FROM batch соединяет виртуальную таблицу с components по uuid
// и обновляет reserved — всё за один round-trip к БД вместо N отдельных UPDATE
//
// Привязка массивов к колонкам позиционная: AS batch(uuid, reserved) назначает алиасы —
// первый массив ($1) → batch.uuid, второй ($2) → batch.reserved
// WHERE c.uuid = batch.uuid использует первую колонку для джойна,
// SET reserved = batch.reserved берёт значение из второй колонки для обновления
func (r *repository) UpdateReservedBatch(ctx context.Context, components []entity.Component) error {
	const query = `
		UPDATE components AS c
		SET reserved   = batch.reserved,
			updated_at = NOW()
		FROM unnest($1::uuid[], $2::int[]) AS batch(uuid, reserved)
		WHERE c.uuid = batch.uuid
	`

	uuids := make([]string, len(components))
	reservedVals := make([]int, len(components))

	for i, c := range components {
		uuids[i] = c.UUID()
		reservedVals[i] = c.Reserved()
	}

	_, err := r.getter.DefaultTrOrDB(ctx, r.pool).Exec(ctx, query, uuids, reservedVals)
	if err != nil {
		return err
	}
	return nil
}
