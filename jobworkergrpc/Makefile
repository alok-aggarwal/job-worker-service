# jobworkergrpc/Makefile

.PHONY: all deps build proto clean run test jobhelper

SERVER_OUT = server
PROTO_DIR = proto/

all: deps build jobhelper

deps:
	@echo "Fetching dependencies for gRPC server..."
	go mod tidy

proto:
	@echo "Generating protobuf files..."
	protoc --go_out=$(PROTO_DIR) --go-grpc_out=$(PROTO_DIR) $(PROTO_DIR)api.proto

build: $(SERVER_OUT)

$(SERVER_OUT): server.go $(PROTO_DIR)/*
	@echo "Building gRPC server..."
	go build -o $(SERVER_OUT) server.go

jobhelper:
	@echo "Building jobhelper binary..."
	@$(MAKE) -C ../jobhelper build


clean:
	@echo "Cleaning gRPC server..."
	rm -f $(SERVER_OUT)

clean_proto:
	@echo "Cleaning protobuf files..."
	rm -rf $(PROTO_DIR)/*.pb.go

run: build
	@echo "Running gRPC server..."
	./$(SERVER_OUT) &

integration-test: deps build
	@echo "Running integration tests..."
	#sudo env PATH=${PATH}:/usr/local/go/bin go test -v ./server_integration_test.go
	go test -v ./server_integration_test.go

specific-test: deps build
	@if [ -z "$(TEST)" ]; then \
                echo "Error: TEST variable is not set. Usage: make specific-test TEST=<TestName>"; \
                exit 1; \
        fi
	@echo "Running specific test: $(TEST)"
	go test -v ./... -run ^$(TEST)$$

