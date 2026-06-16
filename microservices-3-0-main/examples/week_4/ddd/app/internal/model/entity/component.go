package entity

import (
	"time"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
)

// Component — экспортируемый алиас для приватной структуры
type Component = component

// component — агрегат комплектующего с инвариантами:
//   - reserved не может превышать stockQuantity (Available >= 0)
//   - reserved не может стать отрицательным
//
// Приватная структура + публичный алиас: тип доступен из других пакетов,
// но создать entity.Component{} извне нельзя — только через RestoreComponent
// Все поля приватные — защита от прямой модификации
type component struct {
	uuid          string
	name          string
	componentType valueobject.ComponentType
	properties    *valueobject.ComponentProperties
	stockQuantity int
	reserved      int
	createdAt     time.Time
	updatedAt     *time.Time
}

// RestoreComponent восстанавливает компонент из данных хранилища
func RestoreComponent(
	uuid, name string,
	ct valueobject.ComponentType,
	props *valueobject.ComponentProperties,
	stock, reserved int,
	createdAt time.Time,
	updatedAt *time.Time,
) Component {
	return component{
		uuid:          uuid,
		name:          name,
		componentType: ct,
		properties:    props,
		stockQuantity: stock,
		reserved:      reserved,
		createdAt:     createdAt,
		updatedAt:     updatedAt,
	}
}

// Reserve резервирует одну единицу компонента
// Возвращает ErrOutOfStock, если свободных единиц нет
func (c *component) Reserve() error {
	if c.Available() <= 0 {
		return errs.ErrOutOfStock
	}

	c.reserved++

	return nil
}

// Release освобождает одну зарезервированную единицу компонента
// Возвращает ErrNothingToRelease, если резерв равен нулю
func (c *component) Release() error {
	if c.reserved <= 0 {
		return errs.ErrNothingToRelease
	}

	c.reserved--

	return nil
}

// UUID возвращает уникальный идентификатор компонента
func (c *component) UUID() string { return c.uuid }

// Name возвращает название компонента
func (c *component) Name() string { return c.name }

// ComponentType возвращает тип компонента
func (c *component) ComponentType() valueobject.ComponentType { return c.componentType }

// Properties возвращает свойства компонента (Value Object)
func (c *component) Properties() *valueobject.ComponentProperties { return c.properties }

// StockQuantity возвращает общее количество на складе
func (c *component) StockQuantity() int { return c.stockQuantity }

// Reserved возвращает количество зарезервированных единиц
func (c *component) Reserved() int { return c.reserved }

// Available возвращает количество доступных для резервирования единиц
func (c *component) Available() int { return c.stockQuantity - c.reserved }
