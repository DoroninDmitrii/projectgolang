package tests

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx как database/sql драйвер для goose
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	productRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/product"
	productService "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/service/product"
)

const (
	laptopID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001"
	phoneID   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002"
	monitorID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003"
)

var (
	pool *pgxpool.Pool
	svc  *productService.Service
)

// TestMain поднимает PostgreSQL через testcontainers, накатывает миграции и собирает слои приложения
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Читаем креды из .env
	_ = godotenv.Load("../.env")

	// Поднимаем PostgreSQL через testcontainers
	container, err := postgres.Run(
		ctx, os.Getenv("POSTGRES_IMAGE"),
		postgres.WithDatabase(os.Getenv("POSTGRES_DB")),
		postgres.WithUsername(os.Getenv("POSTGRES_USER")),
		postgres.WithPassword(os.Getenv("POSTGRES_PASSWORD")),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		slog.Error("не удалось запустить PostgreSQL контейнер", "error", err)
		os.Exit(1)
	}

	dbURI, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		slog.Error("не удалось получить строку подключения", "error", err)
		os.Exit(1)
	}

	// Накатываем миграции через goose
	sqlDB, err := sql.Open("pgx", dbURI)
	if err != nil {
		slog.Error("не удалось открыть sql.DB", "error", err)
		os.Exit(1)
	}

	if err = goose.SetDialect("postgres"); err != nil {
		slog.Error("не удалось установить диалект goose", "error", err)
		os.Exit(1)
	}

	if err = goose.Up(sqlDB, "../migrations"); err != nil {
		slog.Error("не удалось применить миграции", "error", err)
		os.Exit(1)
	}

	sqlDB.Close()

	// Подключаемся к PostgreSQL через pgxpool
	pool, err = pgxpool.New(ctx, dbURI)
	if err != nil {
		slog.Error("не удалось подключиться к БД", "error", err)
		os.Exit(1)
	}

	// Собираем слои приложения (repository → service)
	repo := productRepo.New(pool)
	svc = productService.New(repo)

	code := m.Run()

	pool.Close()
	_ = container.Terminate(ctx)

	os.Exit(code)
}

func resetProducts(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM products")
	require.NoError(t, err, "не удалось очистить таблицу products")

	_, err = pool.Exec(ctx, `
		INSERT INTO products (id, name, product_type, properties) VALUES
		('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001', 'MacBook Pro 16"', 'laptop',
		 '{"cpu": "Apple M3 Pro", "ram_gb": 36, "ssd_gb": 512}'),
		('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002', 'iPhone 15 Pro', 'phone',
		 '{"screen_size": 6.1, "battery_mah": 3274, "has_nfc": true}'),
		('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003', 'Dell U2723QE', 'monitor',
		 '{"resolution": "3840x2160", "panel_type": "IPS", "refresh_rate_hz": 60}')
	`)
	require.NoError(t, err, "не удалось вставить seed-данные")
}

// TestGet_JSONB_Laptop проверяет десериализацию JSONB для ноутбука
func TestGet_JSONB_Laptop(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	laptop, err := svc.Get(ctx, laptopID)
	require.NoError(t, err, "Get не должен возвращать ошибку")

	assert.Equal(t, "MacBook Pro 16\"", laptop.Name)
	assert.Equal(t, model.ProductType("laptop"), laptop.ProductType)
	assert.Equal(t, "Apple M3 Pro", laptop.Properties.CPU)
	assert.Equal(t, 36, laptop.Properties.RAMGB)
	assert.Equal(t, 512, laptop.Properties.SSDGB)
}

// TestGet_JSONB_Phone проверяет десериализацию JSONB для телефона
func TestGet_JSONB_Phone(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	phone, err := svc.Get(ctx, phoneID)
	require.NoError(t, err)

	assert.Equal(t, model.ProductType("phone"), phone.ProductType)
	assert.InDelta(t, 6.1, phone.Properties.ScreenSize, 0.01)
	assert.Equal(t, 3274, phone.Properties.BatteryMAh)
	assert.True(t, phone.Properties.HasNFC)
}

// TestGet_JSONB_Monitor проверяет десериализацию JSONB для монитора
func TestGet_JSONB_Monitor(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	monitor, err := svc.Get(ctx, monitorID)
	require.NoError(t, err)

	assert.Equal(t, "3840x2160", monitor.Properties.Resolution)
	assert.Equal(t, "IPS", monitor.Properties.PanelType)
	assert.Equal(t, 60, monitor.Properties.RefreshRateHz)
}

// TestCreate_JSONB проверяет создание товара с JSONB-свойствами
func TestCreate_JSONB(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	id, err := svc.Create(ctx, model.ProductInfo{
		Name:        "Test Laptop",
		ProductType: model.ProductTypeLaptop,
		Properties: model.ProductProperties{
			CPU:   "Intel i9",
			RAMGB: 64,
			SSDGB: 2048,
		},
	})
	require.NoError(t, err, "Create не должен возвращать ошибку")
	assert.NotEmpty(t, id, "ID не должен быть пустым")

	// Проверяем, что данные сохранились корректно
	product, err := svc.Get(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, "Test Laptop", product.Name)
	assert.Equal(t, model.ProductTypeLaptop, product.ProductType)
	assert.Equal(t, "Intel i9", product.Properties.CPU)
	assert.Equal(t, 64, product.Properties.RAMGB)
	assert.Equal(t, 2048, product.Properties.SSDGB)
}

// TestUpdateProperties_JSONB проверяет полную замену JSONB-свойств
func TestUpdateProperties_JSONB(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	// Обновляем properties ноутбука
	newProps := model.ProductProperties{
		CPU:   "Apple M4 Max",
		RAMGB: 128,
		SSDGB: 4096,
	}

	err := svc.UpdateProperties(ctx, laptopID, newProps)
	require.NoError(t, err, "UpdateProperties не должен возвращать ошибку")

	// Проверяем, что properties обновились
	laptop, err := svc.Get(ctx, laptopID)
	require.NoError(t, err)

	assert.Equal(t, "Apple M4 Max", laptop.Properties.CPU)
	assert.Equal(t, 128, laptop.Properties.RAMGB)
	assert.Equal(t, 4096, laptop.Properties.SSDGB)
	assert.NotNil(t, laptop.UpdatedAt, "updated_at должен быть заполнен")
}

// TestUpdateProperties_NotFound проверяет ошибку при обновлении несуществующего товара
func TestUpdateProperties_NotFound(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	err := svc.UpdateProperties(ctx, "00000000-0000-0000-0000-000000000000", model.ProductProperties{})
	require.ErrorIs(t, err, errs.ErrProductNotFound)
}

// TestList_JSONB проверяет получение списка товаров с JSONB
func TestList_JSONB(t *testing.T) {
	resetProducts(t)

	ctx := context.Background()

	products, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 3, "должно быть 3 товара")

	// Проверяем, что у каждого товара properties десериализованы
	for _, p := range products {
		switch model.ProductType(p.ProductType) {
		case model.ProductTypeLaptop:
			assert.NotEmpty(t, p.Properties.CPU, "у ноутбука должен быть CPU")
		case model.ProductTypePhone:
			assert.NotZero(t, p.Properties.BatteryMAh, "у телефона должен быть battery_mah")
		case model.ProductTypeMonitor:
			assert.NotEmpty(t, p.Properties.Resolution, "у монитора должен быть resolution")
		}
	}
}
