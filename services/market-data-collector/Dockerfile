# syntax=docker/dockerfile:1

# ====== BUILD STAGE ======
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Копируем все нужные файлы
COPY go.work go.work.sum ./
COPY common ./common
COPY proto ./proto
COPY services ./services


WORKDIR /app/services/market-data-collector

# Сначала тянем зависимости
RUN go mod tidy

# Потом билдим бинарь
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/market-data-collector ./cmd/collector

# ====== RUNTIME STAGE ======
FROM alpine:3.18

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/market-data-collector .
COPY --from=builder /app/services/market-data-collector/config ./config

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN chown -R appuser:appgroup /app
USER appuser

EXPOSE 8080

ENTRYPOINT ["./market-data-collector", "--config", "config/config.yaml"]