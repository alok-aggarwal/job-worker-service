# jobworker/Makefile

.PHONY: all deps build test specific-test integration-test clean jobhelper

LIB_OUT = jobworker.a
SRC = $(wildcard *.go)

export JOB_HELPER_PATH=../jobhelper/jobhelper

all: deps build jobhelper

deps:
	@echo "Fetching dependencies for jobworker library..."
	go mod tidy

build: $(LIB_OUT)

$(LIB_OUT): $(SRC)
	@echo "Building jobworker library..."
	go build -o $(LIB_OUT) ./...

jobhelper:
	@echo "Building jobhelper binary..."
	@$(MAKE) -C ../jobhelper build

test: deps jobhelper
	@echo "Running all tests for jobworker..."
	go test -v ./...

specific-test: deps jobhelper
	@if [ -z "$(TEST)" ]; then \
		echo "Error: TEST variable is not set. Usage: make specific-test TEST=<TestName>"; \
		exit 1; \
	fi
	@echo "Running specific test: $(TEST)"
	sudo env PATH=${PATH}:/usr/local/go/bin go test -v ./... -run ^$(TEST)$$

integration-test: build jobhelper
	@echo "Running integration tests..."
	sudo env PATH=${PATH}:/usr/local/go/bin go test -v ./jobworker_integration_test.go -cover -count=1

clean:
	@echo "Cleaning up jobworker build artifacts..."
	rm -f $(LIB_OUT)
	rm -f ../jobhelper/jobhelper

