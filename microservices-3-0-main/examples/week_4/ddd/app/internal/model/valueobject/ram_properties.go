package valueobject

import (
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
)

// RAMProperties — экспортируемый алиас для приватной структуры
type RAMProperties = ramProperties

// ramProperties — свойства оперативной памяти (Value Object)
type ramProperties struct {
	ramType    string
	capacityGB int
}

// NewRAMProperties создаёт свойства оперативной памяти
// Тип не должен быть пустым, объём должен быть положительным
func NewRAMProperties(ramType string, capacityGB int) (*ComponentProperties, error) {
	if ramType == "" {
		return nil, fmt.Errorf("тип RAM не может быть пустым: %w",
			errs.ErrInvalidProperties)
	}
	if capacityGB <= 0 {
		return nil, fmt.Errorf("объём RAM должен быть положительным: %w",
			errs.ErrInvalidProperties)
	}

	return &componentProperties{
		ram: &ramProperties{
			ramType:    ramType,
			capacityGB: capacityGB,
		},
	}, nil
}

// RAMType возвращает тип оперативной памяти (DDR4/DDR5)
func (r *ramProperties) RAMType() string { return r.ramType }

// CapacityGB возвращает объём оперативной памяти в гигабайтах
func (r *ramProperties) CapacityGB() int { return r.capacityGB }
