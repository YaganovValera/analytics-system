# ========== BUILD STAGE ==========
FROM golang:1.23-alpine AS builder

# Установка зависимостей
RUN apk add --no-cache git ca-certificates

# Рабочая директория для сборки
WORKDIR /app

# Копируем go.mod, go.sum, и go.work если есть
COPY go.mod go.sum ./
# Качаем зависимости
RUN go mod download

# Копируем весь код сервиса (market-data-collector)
COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o market-data-collector ./cmd/collector

# ========== RUNTIME STAGE ==========
FROM alpine:3.18

# Установка сертификатов
RUN apk add --no-cache ca-certificates

# Пользователь для запуска приложения
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Копируем бинарник из builder stage
COPY --from=builder /app/market-data-collector .

# Копируем конфигурацию (если нужно локально поднять)
COPY config/config.yaml ./config/config.yaml

# Меняем владельца файлов
RUN chown -R appuser:appgroup /app

USER appuser

# Экспонируем порт для healthcheck
EXPOSE 8080

# Запуск
ENTRYPOINT ["./market-data-collector", "--config", "config/config.yaml"]
