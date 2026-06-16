package account

import (
	"context"
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/model"
)

// Transfer переводит amount копеек со счёта fromUUID на счёт toUUID
//
// # Безопасная версия: Read Committed + SELECT ... FOR UPDATE
//
// Транзакция работает на уровне изоляции Read Committed (дефолт PostgreSQL)
// Сам по себе Read Committed не защищает от lost update при паттерне read-modify-write
// (см. TransferUnsafe). Поэтому используем SELECT ... FOR UPDATE — он берёт
// эксклюзивную блокировку (row-level exclusive lock) на выбранные строки
// Пока транзакция не завершится, другие транзакции будут ЖДАТЬ на своём SELECT ... FOR UPDATE,
// а не читать устаревшие данные
//
// # Предотвращение deadlock: фиксированный порядок блокировки
//
// Когда нужно заблокировать несколько строк, порядок блокировки критичен
// Без фиксированного порядка два параллельных перевода могут взаимно заблокировать друг друга:
//
//	TX1 (A→B): SELECT ... FOR UPDATE WHERE uuid='A'  — заблокировал A
//	TX2 (B→A): SELECT ... FOR UPDATE WHERE uuid='B'  — заблокировал B
//	TX1 (A→B): SELECT ... FOR UPDATE WHERE uuid='B'  — ждёт, пока TX2 отпустит B
//	TX2 (B→A): SELECT ... FOR UPDATE WHERE uuid='A'  — ждёт, пока TX1 отпустит A → DEADLOCK!
//
// Решение: всегда блокируем строки в одном и том же порядке (здесь — по возрастанию UUID)
// Тогда оба перевода A→B и B→A начнут с блокировки UUID='A', и второй просто подождёт
func (s *Service) Transfer(ctx context.Context, fromUUID, toUUID string, amount int64) error {
	return s.txManager.Do(ctx, func(ctx context.Context) error {
		// 1. Блокируем оба счёта через SELECT ... FOR UPDATE в лексикографическом порядке UUID
		first, second := fromUUID, toUUID
		if fromUUID > toUUID {
			first, second = toUUID, fromUUID
		}

		accounts := make(map[string]model.Account, 2)
		for _, uuid := range [2]string{first, second} {
			acc, err := s.accountRepo.GetForUpdate(ctx, uuid)
			if err != nil {
				return fmt.Errorf("заблокировать счёт: %w", err)
			}

			accounts[uuid] = acc
		}

		from, to := accounts[fromUUID], accounts[toUUID]

		// 2. Проверяем достаточность средств
		if from.Balance < amount {
			return errs.ErrInsufficientFunds
		}

		// 3. Обновляем балансы
		err := s.accountRepo.UpdateBalance(ctx, fromUUID, from.Balance-amount)
		if err != nil {
			return fmt.Errorf("списать со счёта: %w", err)
		}

		err = s.accountRepo.UpdateBalance(ctx, toUUID, to.Balance+amount)
		if err != nil {
			return fmt.Errorf("зачислить на счёт: %w", err)
		}

		return nil
	})
}
