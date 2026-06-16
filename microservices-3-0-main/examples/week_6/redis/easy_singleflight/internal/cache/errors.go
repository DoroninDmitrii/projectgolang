package cache

import "errors"

var (
	// ErrCacheMiss означает, что ключ не найден в кеше
	ErrCacheMiss = errors.New("ключ не найден в кеше")

	// ErrUnexpectedSFResult означает, что singleflight вернул результат неожиданного типа
	ErrUnexpectedSFResult = errors.New("неожиданный тип результата singleflight")
)
