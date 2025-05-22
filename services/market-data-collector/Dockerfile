# // github.com/YaganovValera/analytics-system/services/market-data-collector/Dockerfile
# syntax=docker/dockerfile:1

# ====== BUILD STAGE ======
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# 1) Copy workspace file
COPY go.work go.work.sum ./

# 2) Copy go.mod/go.sum for every module referenced in go.work
COPY common/go.mod common/go.sum ./common/
COPY proto/go.mod proto/go.sum ./proto/
COPY services/query-service/go.mod services/query-service/go.sum ./services/query-service/
COPY services/analytics-api/go.mod services/analytics-api/go.sum ./services/analytics-api/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
COPY services/auth/go.mod services/auth/go.sum ./services/auth/
COPY services/preprocessor/go.mod services/preprocessor/go.sum ./services/preprocessor/
COPY services/market-data-collector/go.mod services/market-data-collector/go.sum ./services/market-data-collector/

# 3) Populate module cache based on go.work
RUN go work sync

# 4) Copy the rest of the source
COPY . .

WORKDIR /app/services/market-data-collector

# 5) Build the static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/market-data-collector ./cmd/collector

# ====== RUNTIME STAGE ======
FROM alpine:3.18 AS runner

RUN apk add --no-cache ca-certificates

WORKDIR /app

# 6) Copy binary and config
COPY --from=builder /app/market-data-collector .
COPY --from=builder /app/services/market-data-collector/config ./config

# 7) Drop privileges
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080

# 8) Entrypoint
ENTRYPOINT ["./market-data-collector", "--config", "config/config.yaml"]
