# services/analytics-api/Dockerfile
# syntax=docker/dockerfile:1

### BUILD STAGE ###
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git

WORKDIR /app

# 1) Кэшируем зависимости
COPY go.work go.work.sum ./
COPY common/go.mod common/go.sum ./common/
COPY proto/go.mod proto/go.sum ./proto/
COPY services/analytics-api/go.mod services/analytics-api/go.sum ./services/analytics-api/
# (при необходимости добавьте другие модули из go.work)
RUN go work sync

# 2) Копируем всё исходники и билдим
COPY . .
WORKDIR /app/services/analytics-api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
    -o /app/analytics-api ./cmd/analytics-api

### RUNTIME STAGE ###
FROM alpine:3.18 AS runner
RUN apk add --no-cache ca-certificates

WORKDIR /app

# 3) Копируем бинарь и конфиг
COPY --from=builder /app/analytics-api .
COPY --from=builder /app/services/analytics-api/config ./config

# 4) Не-root пользователь
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

# 5) Порт gRPC + HTTP метрик/health
EXPOSE 9090

# 6) Запуск
ENTRYPOINT ["./analytics-api", "--config", "config/config.yaml"]
