package valueobject

import (
	"fmt"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
)

// CPUProperties — экспортируемый алиас для приватной структуры
type CPUProperties = cpuProperties

// cpuProperties — свойства процессора (Value Object)
type cpuProperties struct {
	socket   string
	cores    int
	tdpWatts int
}

// NewCPUProperties создаёт свойства процессора
// Сокет не должен быть пустым, TDP и cores должны быть положительными
func NewCPUProperties(socket string, cores, tdpWatts int) (*ComponentProperties, error) {
	if socket == "" {
		return nil, fmt.Errorf("сокет процессора не может быть пустым: %w",
			errs.ErrInvalidProperties)
	}
	if cores <= 0 {
		return nil, fmt.Errorf("количество ядер должно быть положительным: %w",
			errs.ErrInvalidProperties)
	}
	if tdpWatts <= 0 {
		return nil, fmt.Errorf("TDP должен быть положительным: %w",
			errs.ErrInvalidProperties)
	}

	return &componentProperties{
		cpu: &cpuProperties{
			socket:   socket,
			cores:    cores,
			tdpWatts: tdpWatts,
		},
	}, nil
}

// Socket возвращает тип сокета процессора
func (c *cpuProperties) Socket() string { return c.socket }

// Cores возвращает количество ядер процессора
func (c *cpuProperties) Cores() int { return c.cores }

// TDPWatts возвращает тепловыделение процессора в ваттах
func (c *cpuProperties) TDPWatts() int { return c.tdpWatts }
