
# ---------------------------------------------------------------
#  Proto-генерация
# ---------------------------------------------------------------

PROTO_SRC_DIR := proto/v1
PROTO_OUT_DIR := proto/gen/go/
PROTO_FILES   := $(shell find $(PROTO_SRC_DIR) -name '*.proto')

.PHONY: proto-gen
proto-gen:
	@echo "Generating Go code from proto files into $(PROTO_OUT_DIR)..."
	@for file in $(PROTO_FILES); do \
		protoc \
			-Iproto \
			-Ithird_party/googleapis \
			--go_out=paths=source_relative:$(PROTO_OUT_DIR) \
			--go-grpc_out=paths=source_relative:$(PROTO_OUT_DIR) \
			$$file; \
	done

.PHONY: proto-clean
proto-clean:
	@echo "Cleaning generated proto files..."
	@rm -rf $(PROTO_OUT_DIR)
