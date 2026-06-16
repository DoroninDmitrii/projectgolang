package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/model"
	productRepo "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/repository/product"
	productService "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/jsonb/internal/service/product"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ошибка выполнения", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	_ = godotenv.Load(".env") //nolint:gosec // .env файл опционален — ошибка загрузки допустима

	dbURI := os.Getenv("DB_URI")

	pool, err := pgxpool.New(ctx, dbURI)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err = pool.Ping(ctx); err != nil {
		return err
	}

	slog.Info("подключение к PostgreSQL установлено")

	repo := productRepo.New(pool)
	svc := productService.New(repo)

	// 1. Показываем все товары из seed-данных
	fmt.Println("\n═══ Шаг 1: Читаем все товары из seed-данных ═══")

	products, err := svc.List(ctx)
	if err != nil {
		return err
	}

	for _, p := range products {
		printProduct(p)
	}

	// 2. Создаём новый товар с JSONB-свойствами
	fmt.Println("═══ Шаг 2: Создаём новый товар (планшет) ═══")

	newProduct := model.ProductInfo{
		Name:        "iPad Pro 12.9\"",
		ProductType: "tablet",
		Properties: model.ProductProperties{
			ScreenSize: 12.9,
			RAMGB:      16,
			SSDGB:      256,
		},
	}

	newID, err := svc.Create(ctx, newProduct)
	if err != nil {
		return err
	}

	slog.Info("товар создан", "id", newID)

	// 3. Читаем только что созданный товар
	fmt.Println("\n═══ Шаг 3: Читаем созданный товар по ID ═══")

	product, err := svc.Get(ctx, newID)
	if err != nil {
		return err
	}

	printProduct(product)

	// 4. Обновляем JSONB-свойства
	fmt.Println("═══ Шаг 4: Обновляем properties (SSD 256→512, добавляем NFC) ═══")

	err = svc.UpdateProperties(ctx, newID, model.ProductProperties{
		ScreenSize: 12.9,
		RAMGB:      16,
		SSDGB:      512,
		HasNFC:     true,
	})
	if err != nil {
		return err
	}

	// 5. Читаем обновлённый товар
	fmt.Println("\n═══ Шаг 5: Читаем обновлённый товар ═══")

	product, err = svc.Get(ctx, newID)
	if err != nil {
		return err
	}

	printProduct(product)

	return nil
}

// printProduct выводит товар в структурированном виде,
// визуально отделяя обычные колонки от JSONB-поля properties
func printProduct(p model.Product) {
	fmt.Println("┌─────────────────────────────────────────")
	fmt.Printf("│ name:        %s\n", p.Name)
	fmt.Printf("│ type:        %s\n", p.ProductType)
	fmt.Printf("│ created_at:  %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
	if p.UpdatedAt != nil {
		fmt.Printf("│ updated_at:  %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Println("│")
	fmt.Println("│ properties (JSONB):")
	printProps(p.Properties)
	fmt.Println("└─────────────────────────────────────────")
	fmt.Println()
}

// printProps выводит только ненулевые поля JSONB-колонки properties
func printProps(p model.ProductProperties) {
	hasFields := false

	if p.CPU != "" {
		fmt.Printf("│   \"cpu\":            \"%s\"\n", p.CPU)
		hasFields = true
	}
	if p.RAMGB != 0 {
		fmt.Printf("│   \"ram_gb\":         %d\n", p.RAMGB)
		hasFields = true
	}
	if p.SSDGB != 0 {
		fmt.Printf("│   \"ssd_gb\":         %d\n", p.SSDGB)
		hasFields = true
	}
	if p.ScreenSize != 0 {
		fmt.Printf("│   \"screen_size\":    %.1f\n", p.ScreenSize)
		hasFields = true
	}
	if p.BatteryMAh != 0 {
		fmt.Printf("│   \"battery_mah\":    %d\n", p.BatteryMAh)
		hasFields = true
	}
	if p.HasNFC {
		fmt.Printf("│   \"has_nfc\":        true\n")
		hasFields = true
	}
	if p.Resolution != "" {
		fmt.Printf("│   \"resolution\":     \"%s\"\n", p.Resolution)
		hasFields = true
	}
	if p.PanelType != "" {
		fmt.Printf("│   \"panel_type\":     \"%s\"\n", p.PanelType)
		hasFields = true
	}
	if p.RefreshRateHz != 0 {
		fmt.Printf("│   \"refresh_rate_hz\": %d\n", p.RefreshRateHz)
		hasFields = true
	}

	if !hasFields {
		fmt.Println("│   (пусто)")
	}
}
