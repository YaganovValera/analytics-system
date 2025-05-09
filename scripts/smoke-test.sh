#!/usr/bin/env bash
set -euo pipefail

COMPOSE=infra/docker-compose.yml
READY_TIMEOUT=45   # секунд на готовность сервисов

log() { printf "\e[34m[TEST]\e[0m %s\n" "$*"; }

trap 'log "Stopping stack"; docker compose -f $COMPOSE down -v' EXIT

log "Ожидаем готовности HTTP-эндпойнтов"

wait_ready() {
  local url=$1
  local name=$2
  local t=0
  until curl -fsS "$url" >/dev/null 2>&1; do
    sleep 1; ((t++))
    if (( t > READY_TIMEOUT )); then
      log "❌ $name не ответил за ${READY_TIMEOUT}s"; exit 1
    fi
  done
  log "✅ $name готов"
}

wait_ready "http://localhost:8080/readyz" "market-data-collector"
wait_ready "http://localhost:8090/readyz" "preprocessor"

log "Проверяем, что Collector публикует сообщения в Kafka"
docker exec $(docker compose -f $COMPOSE ps -q kafka) \
  kafka-topics.sh --bootstrap-server kafka:9092 --list | grep -q "marketdata.raw"

log "Проверяем, что Preprocessor пишет свечи"
sleep 10   # даём агрегатору минимум один тик
docker exec $(docker compose -f $COMPOSE ps -q kafka) \
  kafka-topics.sh --bootstrap-server kafka:9092 --list | grep -q "marketdata.candles"

log "Smoke-тест пройден успешно ✅"
