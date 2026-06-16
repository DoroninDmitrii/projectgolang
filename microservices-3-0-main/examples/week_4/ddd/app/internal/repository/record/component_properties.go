package record

// ComponentPropertiesRecord — структура для десериализации JSONB из колонки properties
// Живёт в repository-слое: json-теги не принадлежат доменной модели
// Ровно одно поле non-nil — Pointer Union, аналогично доменной модели
type ComponentPropertiesRecord struct {
	Motherboard *MotherboardPropertiesRecord `json:"motherboard,omitempty"`
	CPU         *CPUPropertiesRecord         `json:"cpu,omitempty"`
	RAM         *RAMPropertiesRecord         `json:"ram,omitempty"`
	GPU         *GPUPropertiesRecord         `json:"gpu,omitempty"`
}

// MotherboardPropertiesRecord — JSONB-структура свойств материнской платы
type MotherboardPropertiesRecord struct {
	Socket   string `json:"socket"`
	RAMType  string `json:"ram_type"`
	RAMSlots int    `json:"ram_slots"`
}

// CPUPropertiesRecord — JSONB-структура свойств процессора
type CPUPropertiesRecord struct {
	Socket   string `json:"socket"`
	Cores    int    `json:"cores"`
	TDPWatts int    `json:"tdp_watts"`
}

// RAMPropertiesRecord — JSONB-структура свойств оперативной памяти
type RAMPropertiesRecord struct {
	RAMType    string `json:"ram_type"`
	CapacityGB int    `json:"capacity_gb"`
}

// GPUPropertiesRecord — JSONB-структура свойств видеокарты
type GPUPropertiesRecord struct {
	RequiredTDPWatts int `json:"required_tdp_watts"`
	VRAMGB           int `json:"vram_gb"`
}
