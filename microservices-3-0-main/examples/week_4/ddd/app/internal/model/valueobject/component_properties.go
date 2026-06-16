package valueobject

// ComponentProperties — экспортируемый алиас для приватной структуры
type ComponentProperties = componentProperties

// componentProperties — типоспецифичные свойства комплектующего (Pointer Union Value Object)
// Хранятся в JSONB-колонке. Ровно одно поле non-nil — определяется типом компонента
//
// Приватная структура + публичный алиас: нельзя создать извне напрямую,
// только через конструкторы NewMotherboardProperties, NewCPUProperties и т.д
type componentProperties struct {
	motherboard *MotherboardProperties
	cpu         *CPUProperties
	ram         *RAMProperties
	gpu         *GPUProperties
}

// Motherboard возвращает свойства материнской платы или nil
func (p *componentProperties) Motherboard() *MotherboardProperties { return p.motherboard }

// CPU возвращает свойства процессора или nil
func (p *componentProperties) CPU() *CPUProperties { return p.cpu }

// RAM возвращает свойства оперативной памяти или nil
func (p *componentProperties) RAM() *RAMProperties { return p.ram }

// GPU возвращает свойства видеокарты или nil
func (p *componentProperties) GPU() *GPUProperties { return p.gpu }
