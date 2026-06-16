package pc_builder

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// CreateBuild создаёт сборку ПК из указанных комплектующих
//
// Алгоритм:
//  1. Получает компоненты в транзакции
//  2. Проверяет совместимость через доменный сервис
//  3. Резервирует каждый компонент (доменная логика Reserve)
//  4. Батч-обновляет reserved в БД одним запросом
//  5. Создаёт запись сборки и привязывает компоненты
func (s *Service) CreateBuild(ctx context.Context, componentUUIDs []string) (string, valueobject.BuildStatus, error) {
	var (
		buildUUID = uuid.NewString()
		status    = valueobject.BuildStatusReserved
	)

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		// 1. Получаем компоненты
		components, err := s.componentRepo.List(ctx, componentUUIDs)
		if err != nil {
			return fmt.Errorf("получить компоненты: %w", err)
		}

		if len(components) != len(componentUUIDs) {
			return errs.ErrComponentNotFound
		}

		// 2. Доменная проверка совместимости
		if err = s.compatibilityChecker.Check(components); err != nil {
			return err
		}

		// 3. Резервируем каждый компонент (доменная логика)
		for i := range components {
			if err = components[i].Reserve(); err != nil {
				return fmt.Errorf("резервировать %s: %w", components[i].Name(), err)
			}
		}

		// 4. Батч-обновляем reserved в БД одним запросом
		if err = s.componentRepo.UpdateReservedBatch(ctx, components); err != nil {
			return fmt.Errorf("сохранить резерв: %w", err)
		}

		// 5. Создаём запись сборки (UUID и статус задаются на сервисном слое)
		if err = s.buildRepo.Create(ctx, buildUUID, status); err != nil {
			return fmt.Errorf("создать сборку: %w", err)
		}

		// 6. Привязываем компоненты к сборке
		if err = s.buildRepo.AddComponents(ctx, buildUUID, componentUUIDs); err != nil {
			return fmt.Errorf("добавить компоненты к сборке: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", "", err
	}

	return buildUUID, status, nil
}
