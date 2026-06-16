package errs

import "errors"

var (
	// ErrComponentNotFound — компонент не найден в хранилище
	ErrComponentNotFound = errors.New("компонент не найден")

	// ErrOutOfStock — компонент отсутствует на складе (все зарезервированы или нет в наличии)
	ErrOutOfStock = errors.New("компонент отсутствует на складе")

	// ErrNothingToRelease — нечего освобождать: резерв равен нулю
	ErrNothingToRelease = errors.New("нечего освобождать: резерв равен нулю")

	// ErrIncompatibleSocket — сокет процессора несовместим с материнской платой
	ErrIncompatibleSocket = errors.New("сокет процессора несовместим с материнской платой")

	// ErrIncompatibleRAMType — тип оперативной памяти несовместим с материнской платой
	ErrIncompatibleRAMType = errors.New("тип оперативной памяти несовместим с материнской платой")

	// ErrIncompatibleTDP — видеокарта требует больше мощности, чем процессор может обеспечить
	ErrIncompatibleTDP = errors.New("видеокарта требует больше мощности, чем процессор может обеспечить")

	// ErrBuildAlreadyCancelled — сборка уже отменена
	ErrBuildAlreadyCancelled = errors.New("сборка уже отменена")

	// ErrMotherboardRequired — сборка должна содержать материнскую плату
	ErrMotherboardRequired = errors.New("сборка должна содержать материнскую плату")

	// ErrBuildNotFound — сборка не найдена в хранилище
	ErrBuildNotFound = errors.New("сборка не найдена")

	// ErrInvalidBuildStatus — неизвестный статус сборки
	ErrInvalidBuildStatus = errors.New("неизвестный статус сборки")

	// ErrInvalidComponentType — неизвестный тип компонента
	ErrInvalidComponentType = errors.New("неизвестный тип компонента")

	// ErrInvalidProperties — некорректные свойства компонента (нарушена валидация Value Object)
	ErrInvalidProperties = errors.New("некорректные свойства компонента")
)
