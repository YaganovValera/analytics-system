# syntax=docker/dockerfile:1

# ========================= BUILD STAGE =========================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# 1) go.work + checksum
COPY go.work go.work.sum ./

# 2) go.mod/go.sum для всех модулей, перечисленных в go.work
#    (копируем только файлы, чтобы наполнять кеш модулей максимально дешево)
COPY common/go.mod                 common/go.sum                 ./common/
COPY proto/go.mod                  proto/go.sum                  ./proto/
COPY services/market-data-collector/go.mod services/market-data-collector/go.sum ./services/market-data-collector/
COPY services/preprocessor/go.mod  services/preprocessor/go.sum  ./services/preprocessor/
COPY services/analytics-api/go.mod services/analytics-api/go.sum ./services/analytics-api/
COPY services/api-gateway/go.mod   services/api-gateway/go.sum   ./services/api-gateway/
COPY services/auth/go.mod          services/auth/go.sum          ./services/auth/

# 3) Наполняем module cache по go.work
RUN go work sync

# 4) Копируем остальной исходный код
COPY . .

WORKDIR /app/services/preprocessor

# 5) Сборка статически слинкованного бинаря
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/preprocessor ./cmd/preprocessor

# ========================= RUNTIME STAGE =========================
FROM alpine:3.18 AS runner

RUN apk add --no-cache ca-certificates

WORKDIR /app

# 6) Копируем бинарь и дефолтный конфиг
COPY --from=builder /app/preprocessor .
COPY --from=builder /app/services/preprocessor/config ./config

# 7) Не-root пользователь
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8090

ENTRYPOINT ["./preprocessor", "--config", "config/config.yaml"]
