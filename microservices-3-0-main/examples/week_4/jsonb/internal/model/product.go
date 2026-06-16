package model

import "time"

// ProductType — тип товара
type ProductType string

const (
	ProductTypeLaptop  ProductType = "laptop"
	ProductTypePhone   ProductType = "phone"
	ProductTypeMonitor ProductType = "monitor"
)

// ProductProperties — типоспецифичные свойства товара, хранящиеся в JSONB-колонке
// Поля с omitempty: заполняются только те, что относятся к конкретному типу товара
// Это зеркалит паттерн из домашки — разные типы деталей с разными properties
type ProductProperties struct {
	// Laptop
	CPU   string `json:"cpu,omitempty"`
	RAMGB int    `json:"ram_gb,omitempty"`
	SSDGB int    `json:"ssd_gb,omitempty"`

	// Phone
	ScreenSize float64 `json:"screen_size,omitempty"`
	BatteryMAh int     `json:"battery_mah,omitempty"`
	HasNFC     bool    `json:"has_nfc,omitempty"`

	// Monitor
	Resolution    string `json:"resolution,omitempty"`
	PanelType     string `json:"panel_type,omitempty"`
	RefreshRateHz int    `json:"refresh_rate_hz,omitempty"`
}

// Product — доменная модель товара
type Product struct {
	ID          string
	Name        string
	ProductType ProductType
	Properties  ProductProperties
	CreatedAt   time.Time
	UpdatedAt   *time.Time // nullable
}

// ProductInfo — данные для создания товара (без автоматических полей)
type ProductInfo struct {
	Name        string
	ProductType ProductType
	Properties  ProductProperties
}
