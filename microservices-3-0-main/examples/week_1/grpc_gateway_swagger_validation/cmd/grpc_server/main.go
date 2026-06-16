package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	protovalidateMiddleware "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/protovalidate"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/pkg/proto/ufo/v1"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/static"
)

const (
	// Адреса серверов
	grpcAddress = "localhost:50051"
	httpAddress = ":8081"

	// gRPC keepalive параметры
	grpcMaxConnectionIdle     = 15 * time.Minute // Закрыть idle-соединения (нет активных RPC)
	grpcMaxConnectionAge      = 30 * time.Minute // Принудительная ротация для балансировки
	grpcMaxConnectionAgeGrace = 5 * time.Second  // Время на завершение активных RPC
	grpcKeepaliveTime         = 5 * time.Minute  // Интервал ping'ов для обнаружения мёртвых соединений
	grpcKeepaliveTimeout      = 1 * time.Second  // Таймаут ожидания pong
	grpcMinPingInterval       = 5 * time.Minute  // Минимальный интервал ping'ов от клиента (защита от DoS)

	// HTTP таймауты
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 15 * time.Second
	httpIdleTimeout       = 60 * time.Second
	httpShutdownTimeout   = 5 * time.Second

	// Пути к встроенным файлам
	swaggerUIFile   = "swagger-ui.html"
	swaggerJSONFile = "generated/ufo.swagger.json"
)

// ufoService реализует gRPC сервис для работы с наблюдениями НЛО
type ufoService struct {
	ufov1.UnimplementedUFOServiceServer

	mu        sync.RWMutex
	sightings map[string]*ufov1.Sighting
}

// Create создает новое наблюдение НЛО
// Валидация выполняется автоматически через protovalidate interceptor
func (s *ufoService) Create(ctx context.Context, req *ufov1.CreateRequest) (*ufov1.CreateResponse, error) {
	if req.GetInfo() == nil {
		return nil, status.Error(codes.InvalidArgument, "info не может быть nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Генерируем UUID для нового наблюдения
	newUUID := uuid.NewString()

	sighting := &ufov1.Sighting{
		Uuid:      newUUID,
		Info:      req.GetInfo(),
		CreatedAt: timestamppb.New(time.Now()),
	}

	s.sightings[newUUID] = sighting

	slog.InfoContext(ctx, "создано наблюдение", "uuid", newUUID)

	return &ufov1.CreateResponse{
		Uuid: newUUID,
	}, nil
}

// Get возвращает наблюдение НЛО по UUID
func (s *ufoService) Get(_ context.Context, req *ufov1.GetRequest) (*ufov1.GetResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid обязателен")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	// proto.Clone создаёт глубокую копию, чтобы клиент не получил указатель
	// на объект в хранилище — иначе gRPC может читать его параллельно с записью (data race)
	return &ufov1.GetResponse{
		Sighting: proto.Clone(sighting).(*ufov1.Sighting),
	}, nil
}

// Update обновляет существующее наблюдение НЛО
func (s *ufoService) Update(_ context.Context, req *ufov1.UpdateRequest) (*ufov1.UpdateResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid обязателен")
	}

	if req.GetUpdateInfo() == nil {
		return nil, status.Error(codes.InvalidArgument, "update_info не может быть nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	// Обновляем поля, только если они были установлены в запросе
	if req.GetUpdateInfo().ObservedAt != nil {
		sighting.Info.ObservedAt = req.GetUpdateInfo().ObservedAt
	}

	if req.GetUpdateInfo().Location != nil {
		sighting.Info.Location = req.GetUpdateInfo().Location.Value
	}

	if req.GetUpdateInfo().Description != nil {
		sighting.Info.Description = req.GetUpdateInfo().Description.Value
	}

	if req.GetUpdateInfo().Color != nil {
		sighting.Info.Color = req.GetUpdateInfo().Color
	}

	if req.GetUpdateInfo().Sound != nil {
		sighting.Info.Sound = req.GetUpdateInfo().Sound
	}

	if req.GetUpdateInfo().DurationSeconds != nil {
		sighting.Info.DurationSeconds = req.GetUpdateInfo().DurationSeconds
	}

	sighting.UpdatedAt = timestamppb.New(time.Now())

	return &ufov1.UpdateResponse{}, nil
}

// Delete удаляет наблюдение НЛО (мягкое удаление - устанавливает deleted_at)
func (s *ufoService) Delete(_ context.Context, req *ufov1.DeleteRequest) (*ufov1.DeleteResponse, error) {
	if req.GetUuid() == "" {
		return nil, status.Error(codes.InvalidArgument, "uuid обязателен")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sighting, ok := s.sightings[req.GetUuid()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "наблюдение с UUID %s не найдено", req.GetUuid())
	}

	// Мягкое удаление - устанавливаем deleted_at (идемпотентно: не перезаписываем если уже удалено)
	if sighting.DeletedAt == nil {
		sighting.DeletedAt = timestamppb.New(time.Now())
	}

	return &ufov1.DeleteResponse{}, nil
}

//nolint:funlen // main функция длинная из-за настроек серверов и graceful shutdown
func main() {
	//nolint:noctx // Контекст здесь не нужен: GracefulStop() сам закроет listener и прервёт Accept()
	lis, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		slog.Error("ошибка запуска слушателя", "error", err)
		return
	}
	// Примечание: defer lis.Close() не нужен, так как GracefulStop() закрывает listener автоматически

	// Создаем protovalidate валидатор для проверки входящих запросов
	validator, err := protovalidate.New()
	if err != nil {
		slog.Error("ошибка создания валидатора", "error", err)
		return
	}

	// Создаем gRPC сервер с keepalive настройками и валидацией
	// Подробное описание всех параметров: см. week_1/GRPC_CONNECTIONS.md
	s := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     grpcMaxConnectionIdle,
			MaxConnectionAge:      grpcMaxConnectionAge,
			MaxConnectionAgeGrace: grpcMaxConnectionAgeGrace,
			Time:                  grpcKeepaliveTime,
			Timeout:               grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcMinPingInterval,
			PermitWithoutStream: true, // Разрешить "тёплые" соединения без активных RPC
		}),

		// Interceptor для автоматической валидации входящих запросов через protovalidate
		grpc.ChainUnaryInterceptor(
			protovalidateMiddleware.UnaryServerInterceptor(validator),
		),
	)

	// Регистрируем наш сервис
	service := &ufoService{
		sightings: make(map[string]*ufov1.Sighting),
	}

	ufov1.RegisterUFOServiceServer(s, service)

	// Включаем рефлексию для отладки
	reflection.Register(s)

	// Контекст, который отменяется по SIGINT/SIGTERM или при падении любого из серверов
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Запускаем gRPC сервер в горутине
	go func() {
		slog.Info("🚀 gRPC сервер запущен", "address", grpcAddress)
		if serveErr := s.Serve(lis); serveErr != nil {
			slog.Error("ошибка запуска сервера", "error", serveErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Настраиваем HTTP сервер с gRPC Gateway и Swagger UI до запуска горутины,
	// чтобы избежать race condition при graceful shutdown
	gwCtx, gwCancel := context.WithCancel(context.Background())
	defer gwCancel()

	// Создаем мультиплексор для HTTP запросов
	mux := runtime.NewServeMux()

	// Настраиваем опции для соединения с gRPC сервером
	// TODO: в продакшене необходимо использовать TLS вместо insecure credentials
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// Регистрируем gRPC-gateway хендлеры
	err = ufov1.RegisterUFOServiceHandlerFromEndpoint(
		gwCtx,
		mux,
		grpcAddress,
		opts,
	)
	if err != nil {
		slog.Error("ошибка регистрации gateway", "error", err)
		gwCancel()

		return
	}

	// Создаем HTTP маршрутизатор
	httpMux := http.NewServeMux()

	// Регистрируем API эндпоинты
	httpMux.Handle("/api/", mux)

	// Swagger UI эндпоинты (встроены в бинарник через go:embed)
	httpMux.Handle("/swagger-ui.html", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, readErr := static.FS.ReadFile(swaggerUIFile)
		if readErr != nil {
			http.Error(w, "swagger-ui.html не найден", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, writeErr := w.Write(data); writeErr != nil {
			slog.Error("ошибка записи swagger-ui", "error", writeErr)
		}
	}))
	httpMux.Handle("/swagger.json", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		data, readErr := static.FS.ReadFile(swaggerJSONFile)
		if readErr != nil {
			http.Error(w, "swagger.json не найден", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write(data); writeErr != nil {
			slog.Error("ошибка записи swagger.json", "error", writeErr)
		}
	}))

	// Редирект с корня на Swagger UI
	httpMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/swagger-ui.html", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	}))

	// Создаем HTTP сервер с таймаутами для защиты от атак
	// Подробное описание всех параметров: см. week_1/HTTP_SERVER.md
	gwServer := &http.Server{
		Addr:              httpAddress,
		Handler:           httpMux,
		ReadHeaderTimeout: httpReadHeaderTimeout, // Защита от Slowloris атаки
		ReadTimeout:       httpReadTimeout,       // Лимит на чтение всего запроса
		WriteTimeout:      httpWriteTimeout,      // Лимит на запись ответа
		IdleTimeout:       httpIdleTimeout,       // Таймаут keep-alive соединений
	}

	// Запускаем HTTP сервер в горутине
	go func() {
		slog.Info("🌐 HTTP сервер с gRPC-Gateway и Swagger UI запущен", "address", httpAddress)
		if serveErr := gwServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("ошибка запуска HTTP", "error", serveErr)
			cancel() // будим main, чтобы не висеть бесконечно
		}
	}()

	// Ждём сигнал от ОС или падение любого из серверов
	<-ctx.Done()
	slog.Info("🛑 остановка серверов")

	// Сначала аккуратно останавливаем HTTP сервер
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()
	if shutdownErr := gwServer.Shutdown(shutdownCtx); shutdownErr != nil {
		slog.Error("ошибка остановки HTTP сервера", "error", shutdownErr)
	}
	slog.Info("✅ HTTP сервер остановлен")

	// В конце останавливаем gRPC сервер
	s.GracefulStop()
	slog.Info("✅ gRPC сервер остановлен")
}
