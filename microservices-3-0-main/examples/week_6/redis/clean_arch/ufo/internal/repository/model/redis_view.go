package model

// SightingRedisView — модель для хранения в Redis hash map
//
// Опциональные поля (Color, Sound и т.д.) — указатели с тегом omitempty
// Без omitempty go-redis записывает nil-указатель как "0" или "" в Redis,
// и при чтении обратно указатель становится не nil, а указывает на zero value —
// теряется информация о том, что поле не было задано
// С omitempty nil-поля просто не записываются в hash, и при Scan остаются nil
type SightingRedisView struct {
	UUID         string  `redis:"uuid"`
	ObservedAtNs *int64  `redis:"observed_at,omitempty"`
	Location     string  `redis:"location"`
	Description  string  `redis:"description"`
	Color        *string `redis:"color,omitempty"`
	Sound        *bool   `redis:"sound,omitempty"`
	Duration     *int32  `redis:"duration_sec,omitempty"`
	CreatedAtNs  int64   `redis:"created_at"`
	UpdatedAtNs  *int64  `redis:"updated_at,omitempty"`
	DeletedAtNs  *int64  `redis:"deleted_at,omitempty"`
}
