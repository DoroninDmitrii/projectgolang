package ufo

import (
	"context"
	"fmt"
)

func (s *service) Delete(ctx context.Context, uuid string) error {
	// В реальном приложении здесь может быть бизнес-логика:
	// проверка прав, каскадное удаление связанных данных, уведомления и т.д

	err := s.ufoRepository.Delete(ctx, uuid)
	if err != nil {
		return fmt.Errorf("удалить наблюдение: %w", err)
	}

	return nil
}
