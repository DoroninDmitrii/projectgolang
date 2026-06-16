package domain

import (
	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// componentSet хранит типизированные свойства комплектующих, извлечённые из набора
type componentSet struct {
	motherboard *valueobject.MotherboardProperties
	cpu         *valueobject.CPUProperties
	ram         *valueobject.RAMProperties
	gpu         *valueobject.GPUProperties
}

// CompatibilityChecker — доменный сервис проверки совместимости комплектующих
// Stateless: не хранит состояние, работает только с переданными данными
//
// Правила совместимости:
//  1. Материнская плата — обязательный компонент сборки
//  2. Сокет процессора должен совпадать с сокетом материнской платы
//  3. Тип RAM должен совпадать с типом, поддерживаемым материнской платой
//  4. Требуемая мощность GPU не должна превышать удвоенный TDP (Thermal Design Power) процессора
//     — упрощённая проверка достаточности блока питания
type CompatibilityChecker struct{}

// NewCompatibilityChecker создаёт новый экземпляр доменного сервиса
func NewCompatibilityChecker() *CompatibilityChecker {
	return &CompatibilityChecker{}
}

// Check проверяет совместимость набора комплектующих
func (c *CompatibilityChecker) Check(components []entity.Component) error {
	set := extractComponents(components)

	if err := checkMotherboardRequired(set); err != nil {
		return err
	}

	if err := checkSocket(set); err != nil {
		return err
	}

	if err := checkRAMType(set); err != nil {
		return err
	}

	return checkTDP(set)
}

// extractComponents разбирает срез комплектующих и заполняет componentSet
func extractComponents(components []entity.Component) componentSet {
	var set componentSet

	for _, comp := range components {
		props := comp.Properties()
		if mb := props.Motherboard(); mb != nil {
			set.motherboard = mb
		}
		if cpu := props.CPU(); cpu != nil {
			set.cpu = cpu
		}
		if ram := props.RAM(); ram != nil {
			set.ram = ram
		}
		if gpu := props.GPU(); gpu != nil {
			set.gpu = gpu
		}
	}

	return set
}

// checkMotherboardRequired проверяет, что материнская плата присутствует в сборке
func checkMotherboardRequired(set componentSet) error {
	if set.motherboard == nil {
		return errs.ErrMotherboardRequired
	}

	return nil
}

// checkSocket проверяет совместимость сокетов CPU и материнской платы
func checkSocket(set componentSet) error {
	if set.cpu == nil {
		return nil
	}

	if set.cpu.Socket() != set.motherboard.Socket() {
		return errs.ErrIncompatibleSocket
	}

	return nil
}

// checkRAMType проверяет совместимость типа RAM и материнской платы
func checkRAMType(set componentSet) error {
	if set.ram == nil {
		return nil
	}

	if set.ram.RAMType() != set.motherboard.RAMType() {
		return errs.ErrIncompatibleRAMType
	}

	return nil
}

// checkTDP проверяет, что требуемая мощность GPU не превышает удвоенный TDP CPU
func checkTDP(set componentSet) error {
	if set.cpu == nil || set.gpu == nil {
		return nil
	}

	if set.gpu.RequiredTDPWatts() > set.cpu.TDPWatts()*2 {
		return errs.ErrIncompatibleTDP
	}

	return nil
}
