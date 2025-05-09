# Market Data Collector

`market-data-collector` — высоконагруженный Go-микросервис для сбора рыночных данных с Binance WebSocket, преобразования их в Protobuf и доставки в Kafka с гарантией доставки, мониторингом и трассировкой.

---

## Ключевые возможности

- **Подписка на Binance WebSocket**  
  - Типы потоков: сделки (`trade`) и обновления стакана (`depthUpdate`).
- **Парсинг и сериализация**  
  - JSON → Go → Protobuf (`MarketData` и `OrderBookSnapshot`).
- **Публикация в Kafka**  
  - Топики: `raw` (сделки) и `orderbook` (стакан).  
  - Экспоненциальный backoff, ретраи и метрики публикаций.
- **Self-healing**  
  - Автоматический рестарт WS-коннектора при сбоях или закрытии канала.
- **Backpressure**  
  - Блокирующая отправка в буфер, чтобы не терять сообщения при замедленном потреблении.
- **Наблюдаемость**  
  - **Prometheus**: метрики входящих событий, ошибок парсинга/сериализации, задержек публикаций.  
  - **OpenTelemetry**: spans вокруг ключевых операций (connect, subscribe, readLoop, Process, Publish).  
  - **Логи**: структурированные Zap-логи с trace/request IDs.
- **HTTP-эндпоинты**  
  - `/metrics` — Prometheus handler  
  - `/healthz` — всегда `200 OK`  
  - `/readyz` — проверка доступности Kafka (Ping)
- **Graceful shutdown**  
  - Обработка SIGINT/SIGTERM: корректное завершение WS, Kafka-продьюсера и OTLP-экспортёра.
- **Конфигурация**  
  - YAML + ENV (Viper + Mapstructure), sane defaults, валидация.

---

## Структура репозитория

```
.
├── cmd/collector       # Основной binary
│   └── main.go
├── config              # Пример конфигурации
│   └── config.yaml
├── internal
│   ├── app             # Wiring и главный Run()
│   ├── config          # Загрузка и валидация конфига
│   ├── http            # HTTP-сервис (metrics, healthz, readyz)
│   ├── metrics         # Prometheus-метрики
│   └── processor       # Обработка и маршрутизация событий
├── pkg
│   ├── backoff         # Экспоненциальный retry с метриками
│   ├── binance         # WS-коннектор с self-healing и backpressure
│   ├── kafka           # Kafka-продьюсер с retry, метриками и OTEL
│   ├── logger          # Zap-логгер с Dev/Prod режимами, context-fields
│   └── telemetry       # OpenTelemetry InitTracer + shutdown
├── proto               # Protobuf определения & Go-pkg
├── Dockerfile
└── README.md
```

---

## Быстрый старт

### 1. Клонировать репозиторий

```bash
git clone https://github.com/YaganovValera/analytics-system.git
cd analytics-system/services/market-data-collector
```

### 2. Настроить конфигурацию

Скопируйте `config/config.yaml` и измените по необходимости.

### 3. Сборка и запуск

```bash
go build -o bin/collector ./cmd/collector
./bin/collector --config config/config.yaml
```

Или с Docker:

```bash
docker build -t market-data-collector:latest .
docker run -d   -v $(pwd)/config:/app/config   -p 8080:8080   -e COLLECTOR_KAFKA_BROKERS="kafka:9092"   market-data-collector:latest
```

---

## Мониторинг и трассировка

- **Prometheus**: собирайте метрики с `/metrics`.  
- **Liveness**: `/healthz` → `200 OK`.  
- **Readiness**: `/readyz` → `200 OK` только если Kafka доступен.  
- **OpenTelemetry**: экспорт трассировок в OTLP-консьюмер (`cfg.Telemetry.OTLPEndpoint`).

---

## CI/CD рекомендации

1. **Lint & Vet**: `go vet ./...`, `golangci-lint run`.  
2. **Unit & Integration tests**: `go test -cover ./...`.  
3. **Build & Push**: сборка Docker-образа, публикация в registry.

---

## Лицензия

MIT © YaganovValera