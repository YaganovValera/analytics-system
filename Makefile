# Makefile

# Перечень микросервисов (только имена папок под services/)
GO_SERVICES  := market-data-collector preprocessor auth api-gateway analytics-api

PROTO_DIR   := proto/v1
PROTO_FILES := $(shell find $(PROTO_DIR) -name '*.proto')

.PHONY: proto-gen
proto-gen:
	@echo "Generating Go code from proto files…"
	protoc \
		-I$(PROTO_DIR) \
		--go_out=paths=source_relative:$(PROTO_DIR) \
		--go-grpc_out=paths=source_relative:$(PROTO_DIR) \
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
