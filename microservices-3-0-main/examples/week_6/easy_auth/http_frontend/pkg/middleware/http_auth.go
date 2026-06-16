package middleware

import (
	"net/http"
	"strings"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/auth"
)

// HTTPAuth — HTTP middleware, который извлекает Bearer-токен из заголовка Authorization
// и кладёт его в контекст запроса
//
// Middleware НЕ валидирует сессию — это ответственность gRPC-бэкенда
// HTTP-фронтенд лишь пробрасывает токен дальше через gRPC metadata,
// а бэкенд сам решает, валидна сессия или нет
func HTTPAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "отсутствует заголовок Authorization", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "формат: Bearer <token>", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			http.Error(w, "пустой токен", http.StatusUnauthorized)
			return
		}

		ctx := auth.WithToken(r.Context(), token)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
