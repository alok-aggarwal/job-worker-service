# jobworker/Makefile

.PHONY: all deps build test clean

LIB_OUT = jobworker.a
SRC = $(wildcard *.go)

export JW_HELPER_PATH=../jobhelper/jobhelper

all: deps build

deps:
	@echo "Fetching dependencies for jobworker library..."
	go mod tidy

build: $(LIB_OUT)

$(LIB_OUT): $(SRC)
	@echo "Building jobworker library..."
	go build -o $(LIB_OUT) ./...

test: deps
	@echo "Running tests for jobworker..."
	go test -v ./...


integration-test: build
	@echo "Running integration tests..."
	#sudo go test -v ./jobworker_integration_test.go -cover -count=1
	sudo env PATH=${PATH}:/usr/local/go/bin go test -v ./jobworker_integration_test.go -cover -count=1


clean:
	@echo "Cleaning up jobworker build artifacts..."
	rm -f $(LIB_OUT)
