# auth/Dockerfile
# syntax=docker/dockerfile:1

# ====== BUILD STAGE ======
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# 1. go.work + модули всех сервисов
COPY go.work go.work.sum ./

COPY common/go.mod common/go.sum ./common/
COPY proto/go.mod proto/go.sum ./proto/
COPY services/query-service/go.mod services/query-service/go.sum ./services/query-service/
COPY services/analytics-api/go.mod services/analytics-api/go.sum ./services/analytics-api/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
COPY services/auth/go.mod services/auth/go.sum ./services/auth/
COPY services/preprocessor/go.mod services/preprocessor/go.sum ./services/preprocessor/
COPY services/market-data-collector/go.mod services/market-data-collector/go.sum ./services/market-data-collector/

# 2. Синхронизировать зависимости
RUN go work sync

# 3. Копируем исходники
COPY . .

WORKDIR /app/services/auth

# 4. Сборка бинарника
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/auth ./cmd/auth

# ====== RUNTIME STAGE ======
FROM alpine:3.18 AS runner

RUN apk add --no-cache ca-certificates

WORKDIR /app

# 5. Копируем бинарник и конфиги
COPY --from=builder /app/auth .
COPY --from=builder /app/services/auth/config ./config
COPY --from=builder /app/services/auth/migrations/postgres ./migrations/postgres

# 6. Drop privileges
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8084 8085

# 7. Точка входа
ENTRYPOINT ["./auth", "--config", "config/config.yaml"]
