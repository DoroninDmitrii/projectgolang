package testutil

// Стандартные UUID и цены деталей, вставляемых seed-миграцией
// migrations/inventory/00002_seed_parts.sql.
//
// Тесты зависят от этих значений — если миграция меняется, нужно обновить
// их здесь, а не искать "магические" константы по всему api_test.go.
const (
	HullAluminumUUID   = "550e8400-e29b-41d4-a716-446655440001"
	HullTitaniumUUID   = "550e8400-e29b-41d4-a716-446655440002"
	EngineIonCUUID     = "550e8400-e29b-41d4-a716-446655440003"
	EngineIonBUUID     = "550e8400-e29b-41d4-a716-446655440004"
	ShieldEnergyUUID   = "550e8400-e29b-41d4-a716-446655440005"
	WeaponLaserUUID    = "550e8400-e29b-41d4-a716-446655440006"
	HullOutOfStockUUID = "550e8400-e29b-41d4-a716-446655440007"

	HullAluminumPrice   = 500000
	HullTitaniumPrice   = 1500000
	EngineIonCPrice     = 300000
	EngineIonBPrice     = 800000
	ShieldEnergyPrice   = 400000
	WeaponLaserPrice    = 250000
	HullOutOfStockPrice = 2000000
)
