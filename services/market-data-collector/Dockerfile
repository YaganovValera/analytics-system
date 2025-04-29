# 1. Stage: сборка бинарника
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache ca-certificates git

WORKDIR /app
# Сначала копируем go.work, чтобы Go понял monorepo
COPY go.work go.work.sum ./

# Копируем модули из корня и сам сервис
COPY common/       common/
COPY proto/        proto/
COPY services/market-data-collector/go.mod  services/market-data-collector/go.mod
COPY services/market-data-collector/go.sum  services/market-data-collector/go.sum
COPY services/market-data-collector/         services/market-data-collector/

# Перейдём в каталог сервиса
WORKDIR /app/services/market-data-collector
# Обновим зависимости
RUN go work sync
RUN go mod tidy

# Соберём статический бинарь
RUN CGO_ENABLED=0 GOOS=linux go build -o collector ./cmd/collector

# 2. Stage: минимальный образ
FROM scratch
# Копируем сертификаты
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# Копируем бинарник
COPY --from=builder /app/services/market-data-collector/collector /collector

# Готовимся к монтированию конфигурации
VOLUME ["/config"]
EXPOSE 8080
USER 65532:65532

ENTRYPOINT ["/collector", "--config", "/config/config.yaml"]
