# Market Data Collector

**Service:** `market-data-collector`  
**Purpose:** Подписка на Binance WebSocket, преобразование событий в Protobuf, публикация в Kafka, экспонирование метрик и трассировок.

---

## 🚀 Цель проекта

Построить надёжную, масштабируемую микросервисную платформу для анализа рыночных данных в реальном времени:
1. Подключение к биржевым WS (Binance).
2. Обработка потоковых данных (агрегация OHLC, order book, скользящие средние).
3. Публикация сырых и агрегированных данных в Kafka.
4. Кеширование в Redis (когда потребуется), хранение истории в PostgreSQL (в будущем).
5. API (gRPC/REST) для исторических и текущих метрик.
6. Production-ready практики: liveness/readiness, метрики, трассировки, конфигурируемость, контейнеризация и оркестрация.

---

## 📦 Технологический стек

- Go 1.23  
- gRPC + Protobuf v3  
- REST (health/metrics)  
- Apache Kafka (Sarama + Otelsarama)  
- Redis (будет для кешей)  
- Viper + pflag + mapstructure (конфиг)  
- Zap (логирование)  
- cenkalti/backoff (retry)  
- Gorilla WebSocket  
- Prometheus (client_golang)  
- OpenTelemetry OTLP → Jaeger (tracing)  
- Docker / Docker Compose / Kubernetes (Helm)  

---

## 📂 Структура сервиса

services/market-data-collector/
├── cmd/collector/main.go # точка входа
├── config/config.yaml # пример конфига
├── Dockerfile # multi-stage сборка
├── proto/v1/{analytics,auth,common,marketdata}/*.proto
├── internal/
│ ├── app/collector.go # Run()
│ ├── config/config.go # Load/Validate/Print
│ ├── http/server.go # /metrics, /healthz, /readyz
│ ├── metrics/metrics.go # Prometheus метрики
│ └── processor/processor.go # JSON→Protobuf, Kafka publish
└── pkg/
├── logger/logger.go # zap-wrapper
├── telemetry/otel.go # InitTracer OTLP
├── backoff/backoff.go # retry + metrics
├── kafka/producer.go # SyncProducer + Ping()
└── binance/ws.go # WS connector + backoff


---

## ✅ Что сделано

1. **Конфигурация**  
   - `internal/config` с Viper/ENV/флагами.  
   - Настроены секции: Binance WS, Kafka, Telemetry, Logging, HTTP.

2. **Логирование**  
   - `pkg/logger` на Zap, поддержка контекста (trace_id, request_id).

3. **Трассировки**  
   - `pkg/telemetry.InitTracer` → OTLP-Collector → Jaeger.  
   - Спаны для WS, Processor, Kafka.

4. **Retry / Backoff**  
   - `pkg/backoff` с экспоненциальным backoff, jitter, метрики (retries, failures, successes, delay).

5. **Kafka Producer**  
   - `pkg/kafka.NewProducer` с Sarama SyncProducer, обёрткой OTEL, `Ping()` для readiness.

6. **Binance WS Connector**  
   - `pkg/binance.Connector`: auto-reconnect, ping/pong, backoff, buffer, метрики (connects, drops).

7. **Processor**  
   - `internal/processor.Process`: парсинг trade и depthUpdate, Protobuf, `producer.Publish`, метрики (events, errors, latency).

8. **Метрики Prometheus**  
   - Счётчики и гистограммы в `internal/metrics`.  
   - `/metrics` endpoint через `promhttp`.

9. **HTTP-сервер**  
   - `/healthz` (liveness), `/readyz` (readiness проверяет Kafka via `Ping()`), `/metrics`.

10. **Инструментирование**  
    - Prometheus: `infra/monitoring/prometheus.yml` подхватывает `/metrics`.  
    - Grafana: дашборд вручную создан/импортирован.  
    - OpenTelemetry: Collector конфиг `–config/otel-collector-config.yaml`, Jaeger в Docker Compose.

11. **Контейнеризация**  
    - Multi-stage Dockerfile, non-root user, healthcheck, env-vars.  
    - Docker Compose: Zookeeper, Kafka, OTEL-Collector, Jaeger, Prometheus, Grafana, market-data-collector.

---

## 🛑 Текущий статус и где остановились

- **Бизнес-логика**: полностью реализована и протестирована.  
- **Metrics & Tracing**: сбор метрик и спанов настроен, Grafana и Jaeger работают.  
- **Readiness/Liveness**: `/readyz` проверяет Kafka; WebSocket и Collector-health можно добавить позже.  
- **Отказоустойчивость**: осталось протестировать сценарии падения (Kafka, WS, Collector) и доработать readiness для них.

---

## 📋 Следующие шаги

1. **Тест отказоустойчивости** и доработка readiness для WS/Collector.  
2. **Перейти к сервису `preprocessor`**, копируя и адаптируя текущие best-practices.  
3. **Добавить PostgreSQL** в будущем, когда потребуется история.  

---

> _Этот README поможет быстро вспомнить, что уже сделано в `market-data-collector` и на каком этапе мы остановились перед разработкой следующего сервиса._
