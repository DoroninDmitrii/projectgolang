package ratelimit

import (
	"net/http"
)

// HTTPMiddleware создаёт HTTP middleware с rate limiter
func HTTPMiddleware(limiter Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "слишком много запросов, попробуйте позже", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
