package errors

import "errors"

// ErrAccountNotFound — счёт не найден в БД
var ErrAccountNotFound = errors.New("счёт не найден")

// ErrInsufficientFunds — недостаточно средств для операции
var ErrInsufficientFunds = errors.New("недостаточно средств")
