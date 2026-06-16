package valueobject

import (
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
)

// MotherboardProperties — экспортируемый алиас для приватной структуры
type MotherboardProperties = motherboardProperties

// motherboardProperties — свойства материнской платы (Value Object)
type motherboardProperties struct {
	socket   string
	ramType  string
	ramSlots int
}

// NewMotherboardProperties создаёт свойства материнской платы
// Сокет и тип RAM не должны быть пустыми, количество слотов — положительным
func NewMotherboardProperties(socket, ramType string, ramSlots int) (*ComponentProperties, error) {
	if socket == "" {
		return nil, fmt.Errorf("сокет материнской платы не может быть пустым: %w",
			errs.ErrInvalidProperties)
	}
	if ramType == "" {
		return nil, fmt.Errorf("тип RAM не может быть пустым: %w",
			errs.ErrInvalidProperties)
	}
	if ramSlots <= 0 {
		return nil, fmt.Errorf("количество слотов RAM должно быть положительным: %w",
			errs.ErrInvalidProperties)
	}

	return &componentProperties{
		motherboard: &motherboardProperties{
			socket:   socket,
			ramType:  ramType,
			ramSlots: ramSlots,
		},
	}, nil
}

// Socket возвращает тип сокета материнской платы
func (m *motherboardProperties) Socket() string { return m.socket }

// RAMType возвращает поддерживаемый тип оперативной памяти
func (m *motherboardProperties) RAMType() string { return m.ramType }

// RAMSlots возвращает количество слотов для оперативной памяти
func (m *motherboardProperties) RAMSlots() int { return m.ramSlots }
