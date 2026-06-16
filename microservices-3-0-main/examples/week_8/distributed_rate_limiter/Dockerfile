# Multi-stage сборка: первый этап компилирует бинарник, второй — минимальный образ для запуска.
# Это уменьшает размер финального образа с ~1 ГБ (golang) до ~10 МБ (alpine + бинарник).

# Этап 1: сборка. Используем полный образ Go с компилятором.
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Сначала копируем только файлы зависимостей — Docker кэширует этот слой.
# При изменении кода (но не зависимостей) go mod download не будет выполняться заново.
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь исходный код и компилируем статический бинарник.
# CGO_ENABLED=0 — без привязки к libc, бинарник работает в любом Linux-образе.
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/grpc_server

# Этап 2: запуск. Alpine — минимальный образ (~5 МБ), только для запуска готового бинарника.
FROM alpine:3.21
COPY --from=builder /server /server
ENTRYPOINT ["/server"]
