package converter

import (
	"fmt"

	"github.com/goccy/go-json"

	errs "github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/errors"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/entity"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/model/valueobject"
	"github.com/olezhek28-courses/microservices-course-examples/examples/week_4/ddd/app/internal/repository/record"
)

// ComponentRecordsToModels конвертирует список записей БД в доменные модели
func ComponentRecordsToModels(records []record.ComponentRecord) ([]entity.Component, error) {
	components := make([]entity.Component, 0, len(records))

	for i := range records {
		comp, err := ComponentRecordToModel(&records[i])
		if err != nil {
			return nil, err
		}

		components = append(components, comp)
	}

	return components, nil
}

// ComponentRecordToModel конвертирует запись БД в доменную модель
func ComponentRecordToModel(r *record.ComponentRecord) (entity.Component, error) {
	var propsRec record.ComponentPropertiesRecord
	if err := json.Unmarshal(r.Properties, &propsRec); err != nil {
		return entity.Component{}, fmt.Errorf("десериализовать properties: %w", err)
	}

	props, err := componentPropertiesFromRecord(propsRec)
	if err != nil {
		return entity.Component{}, fmt.Errorf("конвертировать properties: %w", err)
	}

	ct, err := valueobject.NewComponentType(r.Type)
	if err != nil {
		return entity.Component{}, fmt.Errorf("конвертировать тип компонента: %w", err)
	}

	return entity.RestoreComponent(
		r.UUID, r.Name,
		ct,
		props,
		r.StockQuantity, r.Reserved,
		r.CreatedAt, r.UpdatedAt,
	), nil
}

func componentPropertiesFromRecord(rec record.ComponentPropertiesRecord) (*valueobject.ComponentProperties, error) {
	switch {
	case rec.Motherboard != nil:
		return valueobject.NewMotherboardProperties(
			rec.Motherboard.Socket,
			rec.Motherboard.RAMType,
			rec.Motherboard.RAMSlots,
		)
	case rec.CPU != nil:
		return valueobject.NewCPUProperties(
			rec.CPU.Socket,
			rec.CPU.Cores,
			rec.CPU.TDPWatts,
		)
	case rec.RAM != nil:
		return valueobject.NewRAMProperties(
			rec.RAM.RAMType,
			rec.RAM.CapacityGB,
		)
	case rec.GPU != nil:
		return valueobject.NewGPUProperties(
			rec.GPU.RequiredTDPWatts,
			rec.GPU.VRAMGB,
		)
	default:
		return nil, fmt.Errorf("неизвестный тип свойств компонента: %w", errs.ErrInvalidProperties)
	}
}
