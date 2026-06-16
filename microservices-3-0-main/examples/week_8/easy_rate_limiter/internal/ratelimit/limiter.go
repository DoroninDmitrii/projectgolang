package ratelimit

// Limiter определяет интерфейс для rate limiter
type Limiter interface {
	// Allow сообщает, можно ли пропустить запрос прямо сейчас
	Allow() bool
}
