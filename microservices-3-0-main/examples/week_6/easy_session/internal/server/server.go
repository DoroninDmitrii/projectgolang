// Package server реализует gRPC-сервис управления сессиями
package server

import (
	"context"
	"log/slog"
	"net/mail"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/internal/interceptor"
	sessionv1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_6/easy_session/pkg/proto/session/v1"
)

const (
	// bcryptCost — стоимость хеширования bcrypt
	bcryptCost = 10
)

// user представляет пользователя в системе
type user struct {
	Email        string
	Name         string
	PasswordHash string
}

// sessionData хранит данные сессии
type sessionData struct {
	Email string
	Name  string
}

// Server реализует SessionService
type Server struct {
	sessionv1.UnimplementedSessionServiceServer
	mu       sync.RWMutex
	users    map[string]*user        // email -> user
	sessions map[string]*sessionData // token -> session
}

// NewServer создаёт новый экземпляр сервера сессий
func NewServer() *Server {
	return &Server{
		users:    make(map[string]*user),
		sessions: make(map[string]*sessionData),
	}
}

// Register создаёт нового пользователя с хешированием пароля через bcrypt
func (s *Server) Register(ctx context.Context, req *sessionv1.RegisterRequest) (*sessionv1.RegisterResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()
	name := strings.TrimSpace(req.GetName())

	if email == "" || password == "" || name == "" {
		return nil, status.Error(codes.InvalidArgument, "email, password и name обязательны")
	}

	if _, parseErr := mail.ParseAddress(email); parseErr != nil {
		return nil, status.Error(codes.InvalidArgument, "некорректный формат email")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[email]; exists {
		return nil, status.Error(codes.AlreadyExists, "пользователь с таким email уже существует")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		slog.ErrorContext(ctx, "ошибка хеширования пароля", "error", err)
		return nil, status.Error(codes.Internal, "внутренняя ошибка сервера")
	}

	s.users[email] = &user{
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
	}

	slog.InfoContext(ctx, "пользователь зарегистрирован", "email", email)

	return &sessionv1.RegisterResponse{}, nil
}

// Login аутентифицирует пользователя и создаёт сессию
func (s *Server) Login(_ context.Context, req *sessionv1.LoginRequest) (*sessionv1.LoginResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()

	if email == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "email и password обязательны")
	}

	s.mu.RLock()
	u, exists := s.users[email]
	s.mu.RUnlock()

	if !exists {
		return nil, status.Error(codes.Unauthenticated, "неверные учётные данные")
	}

	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "неверные учётные данные")
	}

	token := uuid.New().String()

	s.mu.Lock()
	s.sessions[token] = &sessionData{
		Email: u.Email,
		Name:  u.Name,
	}
	s.mu.Unlock()

	slog.Info("пользователь вошёл в систему", "email", email)

	return &sessionv1.LoginResponse{
		SessionToken: token,
	}, nil
}

// Whoami возвращает информацию о текущем пользователе по токену из контекста
func (s *Server) Whoami(ctx context.Context, _ *sessionv1.WhoamiRequest) (*sessionv1.WhoamiResponse, error) {
	token := interceptor.TokenFromContext(ctx)

	s.mu.RLock()
	sd, exists := s.sessions[token]
	s.mu.RUnlock()

	if !exists {
		return nil, status.Error(codes.Unauthenticated, "сессия не найдена или истекла")
	}

	return &sessionv1.WhoamiResponse{
		Email: sd.Email,
		Name:  sd.Name,
	}, nil
}

// Logout удаляет сессию (идемпотентная операция)
func (s *Server) Logout(ctx context.Context, _ *sessionv1.LogoutRequest) (*sessionv1.LogoutResponse, error) {
	token := interceptor.TokenFromContext(ctx)

	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()

	slog.Info("сессия удалена", "token_prefix", token[:8])

	return &sessionv1.LogoutResponse{}, nil
}
