package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/DoroninDmitrii/projectgolang/project/payment/pkg/app"
)

const grpcAddress = ":50052"

// Параметры keepalive для gRPC сервера
const (
	grpcMaxConnectionIdle     = 15 * time.Minute
	grpcMaxConnectionAge      = 30 * time.Minute
	grpcMaxConnectionAgeGrace = 5 * time.Second
	grpcKeepaliveTime         = 5 * time.Minute
	grpcKeepaliveTimeout      = 1 * time.Second
	grpcMinPingInterval       = 5 * time.Minute
)

func main() {
	if err := run(); err != nil {
		slog.Error("сервис завершился с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var lc net.ListenConfig

	lis, err := lc.Listen(ctx, "tcp", grpcAddress)
	if err != nil {
		return err
	}

	opts := append(app.Interceptors(),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     grpcMaxConnectionIdle,
			MaxConnectionAge:      grpcMaxConnectionAge,
			MaxConnectionAgeGrace: grpcMaxConnectionAgeGrace,
			Time:                  grpcKeepaliveTime,
			Timeout:               grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcMinPingInterval,
			PermitWithoutStream: true,
		}),
	)

	grpcServer := grpc.NewServer(opts...)
	app.RegisterServices(grpcServer)

	// Включаем reflection для postman/grpcurl
	reflection.Register(grpcServer)

	serverErrCh := make(chan error, 1)

	go func() {
		slog.Info("запуск PaymentService", "адрес", grpcAddress)

		if err := grpcServer.Serve(lis); err != nil {
			serverErrCh <- err
		}
	}()

	select {
	case err := <-serverErrCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("получен сигнал остановки, начинаем graceful shutdown")
	grpcServer.GracefulStop()
	slog.Info("сервер остановлен")

	return nil
}
