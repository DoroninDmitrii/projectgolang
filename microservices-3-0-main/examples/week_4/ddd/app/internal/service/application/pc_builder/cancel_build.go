package pc_builder

import (
	"context"
	"fmt"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// CancelBuild отменяет сборку ПК и освобождает зарезервированные компоненты
//
// Алгоритм:
//  1. Получает сборку и проверяет, не отменена ли она уже (доменная логика Cancel)
//  2. Получает компоненты сборки и освобождает резерв (доменная логика Release)
//  3. Батч-обновляет reserved в БД одним запросом
//  4. Обновляет статус сборки на «cancelled»
func (s *Service) CancelBuild(ctx context.Context, buildUUID string) (valueobject.BuildStatus, error) {
	var status valueobject.BuildStatus

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		// 1. Получаем сборку
		build, err := s.buildRepo.Get(ctx, buildUUID)
		if err != nil {
			return fmt.Errorf("получить сборку: %w", err)
		}

		// 2. Доменная проверка: можно ли отменить
		if err = build.Cancel(); err != nil {
			return err
		}

		status = build.Status()

		// 3. Получаем компоненты сборки и освобождаем резерв (доменная логика)
		components, err := s.componentRepo.ListByBuildUUID(ctx, buildUUID)
		if err != nil {
			return fmt.Errorf("получить компоненты сборки: %w", err)
		}

		for i := range components {
			if err = components[i].Release(); err != nil {
				return fmt.Errorf("освободить %s: %w", components[i].Name(), err)
			}
		}

		// 4. Батч-обновляем reserved в БД одним запросом
		if err = s.componentRepo.UpdateReservedBatch(ctx, components); err != nil {
			return fmt.Errorf("сохранить резерв: %w", err)
		}

		// 5. Обновляем статус сборки
		if err = s.buildRepo.UpdateStatus(ctx, build.UUID(), build.Status()); err != nil {
			return fmt.Errorf("обновить статус: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return status, nil
}
