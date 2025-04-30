# syntax=docker/dockerfile:1

# ====== BUILD STAGE ======
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# 1) Сначала копируем только файлы модулей для кэша
COPY go.work go.work.sum ./
COPY common/go.mod common/go.sum ./common/
COPY services/market-data-collector/go.mod services/market-data-collector/go.sum ./services/market-data-collector/

# 2) Скачиваем зависимости всего workspace
RUN go work sync

# 3) Копируем остальной исходник
COPY . .

WORKDIR /app/services/market-data-collector

# 4) Собираем статический бинарь
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/market-data-collector ./cmd/collector

# ====== RUNTIME STAGE ======
FROM alpine:3.18 AS runner

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Копируем бинарь
COPY --from=builder /app/market-data-collector .

# Копируем конфиг
COPY --from=builder /app/services/market-data-collector/config ./config

# Небольшой небезопасный пользователь, без root
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080

ENTRYPOINT ["./market-data-collector", "--config", "config/config.yaml"]
