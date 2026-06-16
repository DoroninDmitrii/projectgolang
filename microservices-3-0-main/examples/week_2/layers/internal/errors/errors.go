package errs

import "errors"

var (
	ErrSightingNotFound   = errors.New("наблюдение не найдено")
	ErrWeatherUnavailable = errors.New("сервис погоды недоступен")
)
