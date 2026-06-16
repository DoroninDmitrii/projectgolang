package account

import (
	"context"
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_3/transaction/internal/errors"
)

// Transfer переводит amount копеек со счёта fromUUID на счёт toUUID
// Операция атомарна: либо оба баланса обновляются, либо ни один
func (s *Service) Transfer(ctx context.Context, fromUUID, toUUID string, amount int64) error {
	// txManager.Do оборачивает весь callback в одну SQL-транзакцию
	// Внутрь callback приходит новый ctx, в котором уже лежит pgx.Tx
	// Все вызовы репозитория ниже получают этот ctx → автоматически работают
	// в рамках одной транзакции, хотя сам репозиторий ничего не знает о транзакциях
	//
	// Если callback вернёт error — произойдёт ROLLBACK (ни одно изменение не применится)
	// Если callback вернёт nil — произойдёт COMMIT (все изменения применятся атомарно)
	return s.txManager.Do(ctx, func(ctx context.Context) error {
		from, err := s.accountRepo.Get(ctx, fromUUID)
		if err != nil {
			return fmt.Errorf("получить счёт отправителя: %w", err)
		}

		if from.Balance < amount {
			return errs.ErrInsufficientFunds
		}

		if err = s.accountRepo.UpdateBalance(ctx, fromUUID, from.Balance-amount); err != nil {
			return fmt.Errorf("списать со счёта: %w", err)
		}

		to, err := s.accountRepo.Get(ctx, toUUID)
		if err != nil {
			return fmt.Errorf("получить счёт получателя: %w", err)
		}

		if err = s.accountRepo.UpdateBalance(ctx, toUUID, to.Balance+amount); err != nil {
			return fmt.Errorf("зачислить на счёт: %w", err)
		}

		// nil → COMMIT: оба баланса обновлены атомарно
		return nil
	})
}
