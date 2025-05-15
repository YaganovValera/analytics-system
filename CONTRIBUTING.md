# Analytics System

## Описание проекта
Analytics System — облачная платформа для сбора, обработки и визуализации рыночных данных в режиме реального времени.

## Структура репозитория
```
.
├── common/               # Общие утилиты и клиенты (logger, backoff, kafka, httpserver, telemetry)
├── proto/                # Определения gRPC+Protobuf (v1 и далее)
└── services/
    ├── market-data-collector/
    ├── preprocessor/
    ├── analytics-api/
    ├── api-gateway/
    ├── auth/
    ├── alerter/
    └── query-service/
```

## Основные микросервисы
- **market-data-collector**  
  — Собирает live-данные через WebSocket (Binance и др.), оборачивает в Protobuf, публикует в Kafka.  
- **preprocessor**  
  — Читает «сырые» события из Kafka, поддерживает частичные бары в Redis (по всем интервалам из `common.proto`), агрегирует их (1 м, 5 м, 15 м, 1 ч, 4 ч, 1 д) и публикует готовые свечи обратно в Kafka.  
- **analytics-api**  
  — gRPC-сервис для исторических и стриминговых OHLCV-данных.  
- **api-gateway**  
  — HTTP/REST + gRPC-шлюз, маршрутизация, аутентификация/авторизация.  
- **auth**  
  — JWT-аутентификация, refresh/revoke токенов, проверка валидности.  
- **alerter**  
  — Мониторинг метрик и отправка уведомлений при порогах.  
- **query-service**  
  — Ad-hoc HTTP/gRPC-запросы к историческим данным в БД.

## Используемый стек
- **Go 1.23**  
- **Docker**: `golang:1.23-alpine`  
- **gRPC & Protobuf** (protoc v3.21.x, protoc-gen-go v1.36.6)  
- **Kafka**: Sarama v1.45.1, otelsarama  
- **Redis**: go-redis/redis v8.11.5  - по желанию 
- **PostgreSQL**: pgx/v5.5.0 или lib/pq v1.13.0  и обязательно TimescaleDB 
- **Zap** v1.27.0  
- **OpenTelemetry** v1.35.0  
- **Prometheus client_golang** v1.22.0  
- **Viper**, **pflag**, **mapstructure**  
- **cenkalti/backoff/v4**, **errgroup**  
- **Docker Compose**, **Kubernetes**, **Helm**

## Правила разработки микросервисов
1. **ВАЖНО: Продакшен-уровень**  
   — Без MVP, демо и заглушек. Код должен сразу соответствовать best practices.  
2. **Метрики и трассинг**  
   — Все внешние операции (Kafka, Redis, WebSocket) оборачиваются в backoff + OpenTelemetry spans + Prometheus.  
3. **Интерфейсы**  
   — Все внешние зависимости (Kafka, Redis и т.д.) — через Go-интерфейсы для простоты тестирования.  
4. **Порядок разработки**  
   1) Реализуем `pkg/` / `common/`  
   2) `internal/`  
   3) `cmd/` + `Dockerfile`  
5. **Мне очень важно чтобы код был чистым, надежным и удовлетворял всем best-практикам и правилам Go. Поэтому, если ты видишь, что какое то решение выглядит странно или требует доработки. То следуй данному правилу:** 
  - сообщи об этой неточности;
  - скажи почему лучше выбрать другой вариант и чем это критично; 
  - предоставь исправленный вариант;
  - если эти изменения затрагивают другие файлы, то обязательно сообщи об этом и скажи, как их подправить. 


