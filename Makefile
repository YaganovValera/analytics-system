
# ---------------------------------------------------------------
#  Proto-генерация
# ---------------------------------------------------------------
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

