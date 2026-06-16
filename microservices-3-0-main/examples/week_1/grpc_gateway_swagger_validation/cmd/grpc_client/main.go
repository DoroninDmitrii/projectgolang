package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	ufov1 "github.com/olezhek28-courses/microservices-course-examples/examples/week_1/grpc_gateway_swagger_validation/pkg/proto/ufo/v1"
)

const serverAddress = "localhost:50051"

// createSighting создает новое наблюдение НЛО с рандомными данными
func createSighting(ctx context.Context, client ufov1.UFOServiceClient) (string, error) {
	// Генерируем случайные данные с помощью gofakeit
	observedAt := gofakeit.DateRange(
		time.Now().AddDate(-3, 0, 0), // за последние 3 года
		time.Now(),
	)
	location := gofakeit.City() + ", " + gofakeit.StreetName()
	description := gofakeit.Sentence(10)

	// Создаем базовую информацию о наблюдении
	info := &ufov1.SightingInfo{
		ObservedAt:  timestamppb.New(observedAt),
		Location:    location,
		Description: description,
	}

	// Иногда добавляем дополнительные поля (с вероятностью 50%)
	if gofakeit.Bool() {
		info.Color = wrapperspb.String(gofakeit.Color())
	}

	if gofakeit.Bool() {
		info.Sound = wrapperspb.Bool(gofakeit.Bool())
	}

	if gofakeit.Bool() {
		info.DurationSeconds = wrapperspb.Int32(int32(gofakeit.IntRange(1, 3600))) //nolint:gosec // диапазон 1-3600 безопасен для int32
	}

	// Вызываем gRPC метод Create
	resp, err := client.Create(ctx, &ufov1.CreateRequest{Info: info})
	if err != nil {
		return "", err
	}

	return resp.Uuid, nil
}

// getSighting получает информацию о наблюдении по UUID
func getSighting(ctx context.Context, client ufov1.UFOServiceClient, uuid string) (*ufov1.Sighting, error) {
	resp, err := client.Get(ctx, &ufov1.GetRequest{Uuid: uuid})
	if err != nil {
		return nil, err
	}

	return resp.Sighting, nil
}

// updateSighting обновляет наблюдение НЛО
func updateSighting(ctx context.Context, client ufov1.UFOServiceClient, uuid string) error {
	// Генерируем рандомные данные для обновления
	updateInfo := &ufov1.SightingUpdateInfo{}

	// Обновляем часть полей случайным образом
	if gofakeit.Bool() {
		updateInfo.ObservedAt = timestamppb.New(gofakeit.DateRange(
			time.Now().AddDate(-3, 0, 0),
			time.Now(),
		))
	}

	if gofakeit.Bool() {
		location := gofakeit.City() + ", " + gofakeit.StreetName()
		updateInfo.Location = wrapperspb.String(location)
	}

	if gofakeit.Bool() {
		description := gofakeit.Sentence(10)
		updateInfo.Description = wrapperspb.String(description)
	}

	if gofakeit.Bool() {
		updateInfo.Color = wrapperspb.String(gofakeit.Color())
	}

	if gofakeit.Bool() {
		updateInfo.Sound = wrapperspb.Bool(gofakeit.Bool())
	}

	if gofakeit.Bool() {
		updateInfo.DurationSeconds = wrapperspb.Int32(int32(gofakeit.IntRange(1, 3600))) //nolint:gosec // диапазон 1-3600 безопасен для int32
	}

	// Вызываем gRPC метод Update
	_, err := client.Update(ctx, &ufov1.UpdateRequest{
		Uuid:       uuid,
		UpdateInfo: updateInfo,
	})
	if err != nil {
		return err
	}

	return nil
}

// deleteSighting удаляет наблюдение НЛО
func deleteSighting(ctx context.Context, client ufov1.UFOServiceClient, uuid string) error {
	_, err := client.Delete(ctx, &ufov1.DeleteRequest{Uuid: uuid})
	if err != nil {
		return err
	}

	return nil
}

func main() {
	ctx := context.Background()

	// Создаем gRPC соединение с keepalive настройками
	// Подробное описание всех параметров: см. week_1/GRPC_CONNECTIONS.md
	conn, err := grpc.NewClient(
		serverAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // Интервал ping'ов для обнаружения мёртвых соединений
			Timeout:             3 * time.Second,  // Таймаут ожидания pong
			PermitWithoutStream: true,             // Держать соединение "тёплым" без активных RPC
		}),
	)
	if err != nil {
		slog.Error("ошибка подключения", "error", err)
		return
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			slog.Error("ошибка закрытия соединения", "error", cerr)
		}
	}()

	// Создаем gRPC клиент
	client := ufov1.NewUFOServiceClient(conn)

	slog.Info("=== тестирование API для работы с наблюдениями НЛО ===")
	slog.Info("")

	// 1. Создаем несколько наблюдений
	slog.Info("🛸 создание наблюдений НЛО")
	slog.Info("===========================")
	uuid, err := createSighting(ctx, client)
	if err != nil {
		slog.Error("ошибка при создании наблюдения", "error", err)
		return
	}

	// Выводим информацию о созданном наблюдении
	slog.Info("создано наблюдение НЛО", "uuid", uuid)

	// 2. Получаем информацию о наблюдении
	slog.Info("🔍 получение информации о наблюдении")
	slog.Info("==================================")
	sighting, err := getSighting(ctx, client, uuid)
	if err != nil {
		slog.Error("ошибка при получении наблюдения", "error", err)
		return
	}

	// Выводим информацию о полученном наблюдении
	slog.Info("получено наблюдение НЛО", "uuid", uuid, "sighting", sighting)

	// 3. Обновляем наблюдение
	slog.Info("✏️ обновление наблюдения")
	slog.Info("=======================")

	err = updateSighting(ctx, client, uuid)
	if err != nil {
		slog.Error("ошибка при обновлении наблюдения", "error", err)
		return
	}

	// 4. Проверяем обновленное наблюдение
	slog.Info("🔍 проверка обновленного наблюдения")
	slog.Info("=================================")
	updatedSighting, err := getSighting(ctx, client, uuid)
	if err != nil {
		slog.Error("ошибка при получении обновленного наблюдения", "error", err)
		return
	}

	// Выводим информацию об обновленном наблюдении
	slog.Info("получено обновленное наблюдение НЛО", "uuid", uuid, "sighting", updatedSighting)

	// 5. Удаляем наблюдение
	err = deleteSighting(ctx, client, uuid)
	if err != nil {
		slog.Error("ошибка при удалении наблюдения", "error", err)
	}

	// 6. Проверяем удаленное наблюдение
	slog.Info("🔍 проверка удаленного наблюдения")
	slog.Info("=================================")
	deletedSighting, err := getSighting(ctx, client, uuid)
	if err != nil {
		slog.Error("ошибка при получении удаленного наблюдения", "error", err)
		return
	}

	// Выводим информацию об удаленном наблюдении
	slog.Info("получено удаленное наблюдение НЛО", "uuid", uuid, "sighting", deletedSighting)

	slog.Info("тестирование завершено!")
}
