# order.Dockerfile — HTTP-сервис (OrderService).
# Multi-stage сборка: тяжёлый Go-компилятор только на этапе build,
# в финальный образ попадает только скомпилированный бинарник.
#
# Версии образов передаются через build args из core.env, чтобы не
# дублировать их в каждом Dockerfile. Defaults на случай ручного `docker build`.
ARG GO_IMAGE=golang:1.26-alpine3.23
ARG ALPINE_IMAGE=alpine:3.23

FROM ${GO_IMAGE} AS builder

WORKDIR /app

# Копируем файлы Go workspace и все go.mod/go.sum.
# Отдельным слоем — Docker кеширует его, пока зависимости не изменятся.
COPY go.work go.work.sum ./
COPY platform/go.mod platform/go.sum ./platform/
COPY shared/go.mod shared/go.sum ./shared/
COPY order/go.mod order/go.sum ./order/
COPY inventory/go.mod inventory/go.sum ./inventory/
COPY payment/go.mod payment/go.sum ./payment/
COPY iam/go.mod iam/go.sum ./iam/
COPY assembly/go.mod assembly/go.sum ./assembly/

RUN go work sync

# Копируем исходный код.
COPY . .

# Собираем бинарник. CGO_ENABLED=0 — статическая линковка,
# не нужны libc и другие системные библиотеки в runtime-образе.
RUN CGO_ENABLED=0 go build -o /bin/service ./order/cmd/main.go

# --- Runtime ---
# alpine — минимальный образ (~5 MB), достаточный для запуска Go-бинарника.
FROM ${ALPINE_IMAGE}

COPY --from=builder /bin/service /bin/service

ENTRYPOINT ["/bin/service"]
