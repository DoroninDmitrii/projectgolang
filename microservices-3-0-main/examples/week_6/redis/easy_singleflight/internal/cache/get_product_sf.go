package cache

import (
	"context"
	"errors"
	"log/slog"
)

// GetProductSF получает товар с защитой через singleflight
// При cache miss только одна горутина идёт в БД, остальные ждут её результат
func (c *Cache) GetProductSF(ctx context.Context, productID string, fetchFn FetchFunc) (*Product, error) {
	// Пробуем кеш
	product, err := c.Get(ctx, productID)
	if nil == err {
		return product, nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return nil, err
	}

	// Cache miss — используем singleflight
	//
	// Как работает singleflight под капотом:
	//   1. Внутри — map[string]*call, защищённый мьютексом
	//   2. Первый вызов Do(key) создаёт запись в map и запускает fn
	//   3. Все последующие вызовы Do с тем же key видят, что запись уже есть,
	//      и блокируются на WaitGroup (wg.Wait), пока fn не завершится
	//   4. Когда fn завершается — результат и ошибка сохраняются в *call,
	//      запись удаляется из map, и wg.Done() разблокирует всех ожидающих
	//   5. Все горутины получают один и тот же результат (или ту же ошибку)
	//
	// Do возвращает 3 значения: (result, err, shared)
	// shared == true означает, что эта горутина НЕ выполняла fn сама,
	// а дождалась результата от другой горутины. Полезно для метрик,
	// но здесь мы его игнорируем
	result, err, _ := c.sf.Do("product:"+productID, func() (any, error) {
		p, fetchErr := fetchFn(ctx, productID)
		if fetchErr != nil {
			return nil, fetchErr
		}

		// Записываем в кеш
		if cacheErr := c.Set(ctx, p); cacheErr != nil {
			slog.Warn("не удалось записать в кеш", "error", cacheErr)
		}

		return p, nil
	})

	if err != nil {
		return nil, err
	}

	product, ok := result.(*Product)
	if !ok {
		return nil, ErrUnexpectedSFResult
	}

	return product, nil
}
