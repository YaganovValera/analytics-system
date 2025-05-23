# services/analytics-api/Dockerfile
# syntax=docker/dockerfile:1

# ====== BUILD STAGE ======
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app

# 1) Copy workspace
COPY go.work go.work.sum ./

# 2) Copy all go.mod/go.sum
COPY common/go.mod common/go.sum ./common/
COPY proto/go.mod proto/go.sum ./proto/
COPY services/query-service/go.mod services/query-service/go.sum ./services/query-service/
COPY services/analytics-api/go.mod services/analytics-api/go.sum ./services/analytics-api/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
COPY services/auth/go.mod services/auth/go.sum ./services/auth/
COPY services/preprocessor/go.mod services/preprocessor/go.sum ./services/preprocessor/
COPY services/market-data-collector/go.mod services/market-data-collector/go.sum ./services/market-data-collector/

# 3) Sync modules
RUN go work sync

# 4) Copy source
COPY . .
WORKDIR /app/services/analytics-api

# 5) Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/analytics-api ./cmd/analytics-api

# ====== RUNTIME STAGE ======
FROM alpine:3.18 AS runner
RUN apk add --no-cache ca-certificates
WORKDIR /app

# 6) Copy binary and config
COPY --from=builder /app/analytics-api .
COPY --from=builder /app/services/analytics-api/config ./config

# 7) Drop privileges
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8082 8083

# 8) Entrypoint
ENTRYPOINT ["./analytics-api", "--config", "config/config.yaml"]
