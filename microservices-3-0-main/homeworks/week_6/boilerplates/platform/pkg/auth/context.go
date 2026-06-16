package auth

import (
	"context"

	"github.com/google/uuid"
)

type (
	userUUIDCtxKey    struct{}
	sessionUUIDCtxKey struct{}
)

// WithUserUUID кладёт UUID пользователя в контекст
func WithUserUUID(ctx context.Context, userUUID uuid.UUID) context.Context {
	return context.WithValue(ctx, userUUIDCtxKey{}, userUUID)
}

// UserUUIDFromContext извлекает UUID пользователя из контекста
func UserUUIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userUUID, ok := ctx.Value(userUUIDCtxKey{}).(uuid.UUID)
	return userUUID, ok
}

// WithSessionUUID кладёт UUID сессии в контекст.
// Хранится как строка — это чистый passthrough из заголовка/metadata в исходящие
// gRPC metadata через SessionForwarder; парсинг здесь не нужен
func WithSessionUUID(ctx context.Context, sessionUUID string) context.Context {
	return context.WithValue(ctx, sessionUUIDCtxKey{}, sessionUUID)
}

// SessionUUIDFromContext извлекает UUID сессии из контекста
func SessionUUIDFromContext(ctx context.Context) (string, bool) {
	sessionUUID, ok := ctx.Value(sessionUUIDCtxKey{}).(string)
	return sessionUUID, ok
}
