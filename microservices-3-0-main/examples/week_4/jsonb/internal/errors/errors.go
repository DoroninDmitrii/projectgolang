package errors

import "errors"

// ErrProductNotFound — товар не найден в БД
var ErrProductNotFound = errors.New("товар не найден")
