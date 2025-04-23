# Makefile

# Каталог с proto-файлами
PROTO_DIR    := proto/v1
PROTO_FILES  := $(wildcard $(PROTO_DIR)/*.proto)

# Перечень микросервисов (только имена папок под services/)
GO_SERVICES  := market-data-collector preprocessor auth api-gateway analytics-api

# --------------------------------------------------
# Генерация Go-кода из .proto
.PHONY: proto-gen
proto-gen:
	protoc \
	  --proto_path=$(PROTO_DIR) \
	  --go_out=./$(PROTO_DIR)/generate --go_opt=paths=source_relative \
	  --go-grpc_out=./$(PROTO_DIR)/generate --go-grpc_opt=paths=source_relative \
	  $(PROTO_FILES)

# --------------------------------------------------
# Сборка всех сервисов (зависит от proto-gen)
.PHONY: build
build: proto-gen
	@mkdir -p bin
	@for svc in $(GO_SERVICES); do \
	  echo "Building $$svc..."; \
	  go build -o bin/$$svc ./services/$$svc/cmd/$$svc; \
	done

# --------------------------------------------------
# Запуск тестов по каждому сервису
.PHONY: test
test:
	@for svc in $(GO_SERVICES); do \
	  echo "Testing $$svc..."; \
	  go test ./services/$$svc/...; \
	done

# --------------------------------------------------
# (Опционально) Линтинг proto — если используете buf
.PHONY: proto-lint
proto-lint:
	buf lint

# --------------------------------------------------
# Полная CI-цель
.PHONY: ci
ci: proto-lint build test
