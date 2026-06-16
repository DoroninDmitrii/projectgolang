package session

import "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_auth/shared/pkg/auth"

// Store — in-memory хранилище сессий для демонстрации (token -> User)
// В реальном приложении это Redis, Memcached или БД
var Store = map[string]auth.User{
	"token-admin-123":  {ID: "1", Email: "admin@example.com", Role: "admin"},
	"token-user-456":   {ID: "2", Email: "user@example.com", Role: "user"},
	"token-viewer-789": {ID: "3", Email: "viewer@example.com", Role: "viewer"},
}
