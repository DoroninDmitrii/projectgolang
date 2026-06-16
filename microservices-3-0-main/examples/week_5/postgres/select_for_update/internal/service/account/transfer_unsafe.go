package account

import (
	"context"
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_5/postgres/select_for_update/internal/errors"
)

// TransferUnsafe переводит amount копеек со счёта fromUUID на счёт toUUID
//
// ⚠️ НЕБЕЗОПАСНАЯ версия: использует обычный SELECT (без FOR UPDATE)
//
// # Уровень изоляции: Read Committed (дефолт PostgreSQL)
//
// Транзакция открывается на уровне Read Committed. Его ключевое свойство:
// каждый отдельный SQL-оператор (SELECT, UPDATE) видит только те данные, которые были
// закоммичены на момент начала ЭТОГО ОПЕРАТОРА, а не на момент начала транзакции
//
// Это значит, что Read Committed НЕ защищает от аномалии «потерянное обновление» (lost update),
// если между SELECT и UPDATE нет явной блокировки строки
//
// # Почему Read Committed без FOR UPDATE — это проблема
//
// Паттерн read-modify-write (прочитать баланс → вычислить новый → записать) ломается,
// когда две транзакции читают одно и то же значение до того, как кто-то из них успел обновить:
//
//	TX1: BEGIN
//	TX1: SELECT balance FROM accounts WHERE uuid='A'  → 1000
//	TX2: BEGIN
//	TX2: SELECT balance FROM accounts WHERE uuid='A'  → 1000  (строка НЕ заблокирована, читаем те же 1000)
//	TX1: UPDATE accounts SET balance = 900 WHERE uuid='A'      (1000 - 100)
//	TX1: COMMIT
//	TX2: UPDATE accounts SET balance = 900 WHERE uuid='A'      (1000 - 100, но к этому моменту реальный баланс уже 900!)
//	TX2: COMMIT
//
// Итог: два списания по 100, но баланс уменьшился только на 100 вместо 200.
// UPDATE во второй транзакции перезаписал результат первой — это и есть lost update
//
// Обратите внимание: UPDATE в TX2 дождётся COMMIT от TX1 (т.к. UPDATE берёт row-level lock),
// но к этому моменту TX2 уже приняла решение на основе устаревших данных (balance = 1000)
// Read Committed гарантирует, что UPDATE увидит закоммиченную версию строки, но НЕ перечитает
// баланс — он просто применит SET balance = 900, а не пересчитает значение
//
// # Альтернатива: Serializable
//
// Уровень изоляции Serializable решил бы проблему без явных блокировок — PostgreSQL
// автоматически обнаружил бы конфликт и откатил одну из транзакций с ошибкой
// serialization_failure (40001). Но у Serializable свои минусы:
//   - нужна логика повторных попыток (retry loop) при serialization_failure;
//   - выше overhead: PostgreSQL отслеживает зависимости между транзакциями (SSI — Serializable Snapshot Isolation);
//   - при высокой конкуренции за одни и те же строки — частые откаты, что сильно бьёт по throughput
//
// # Правильное решение: Read Committed + SELECT ... FOR UPDATE
//
// Вместо повышения уровня изоляции достаточно явно заблокировать строку при чтении:
// SELECT ... FOR UPDATE не даст другой транзакции прочитать строку, пока блокировка не снята
// Вторая транзакция будет ЖДАТЬ на SELECT, а не читать устаревшие данные
// См. безопасную версию: Transfer()
func (s *Service) TransferUnsafe(ctx context.Context, fromUUID, toUUID string, amount int64) error {
	return s.txManager.Do(ctx, func(ctx context.Context) error {
		// Обычный SELECT — НЕ блокирует строку
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

		return nil
	})
}
