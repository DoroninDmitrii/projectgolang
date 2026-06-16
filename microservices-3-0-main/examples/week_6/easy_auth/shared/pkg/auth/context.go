package auth

import "context"

// SessionTokenKey — ключ metadata для передачи session-токена между HTTP и gRPC
const SessionTokenKey = "session-token"

// User представляет аутентифицированного пользователя
type User struct {
	ID    string
	Email string
	Role  string
}

// ctxKey — неэкспортируемый тип для ключей контекста
// Используется int, а не string, потому что:
//   - безопасность от коллизий обеспечивает сам тип, а не значение внутри;
//   - сравнение int — одна машинная инструкция без аллокаций;
//   - iota автоматически генерирует уникальные значения
type ctxKey int

const (
	userKey  ctxKey = iota // ключ для хранения User в контексте
	tokenKey               // ключ для хранения токена сессии в контексте
)

// WithUser добавляет пользователя в контекст
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFromContext извлекает пользователя из контекста
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}

// WithToken добавляет токен сессии в контекст
//
// Важно понимать, как устроен context.WithValue внутри
// Контекст — это НЕ плоская map[key]value. Каждый вызов WithValue оборачивает
// родительский контекст в новый слой (структуру valueCtx):
//
//	type valueCtx struct {
//	    Context          // родительский контекст (вложенный)
//	    key, val any     // одна пара ключ-значение
//	}
//
// Получается односвязный список (linked list). Например:
//
//	ctx0 := context.Background()
//	ctx1 := context.WithValue(ctx0, "a", 1)
//	ctx2 := context.WithValue(ctx1, "b", 2)
//	ctx3 := context.WithValue(ctx2, "c", 3)
//
// В памяти это выглядит так:
//
//	ctx3 {key:"c", val:3} → ctx2 {key:"b", val:2} → ctx1 {key:"a", val:1} → ctx0 (empty)
//
// При вызове ctx3.Value("a") Go идёт по цепочке: ctx3 → ctx2 → ctx1 — нашёл!
// Поиск — O(n), где n — количество вложенных WithValue
//
// Поэтому контекст не предназначен для хранения десятков значений —
// только для request-scoped данных (user, token, request-id, trace-id)
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey, token)
}

// TokenFromContext извлекает токен сессии из контекста
func TokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(tokenKey).(string)
	return t, ok
}
