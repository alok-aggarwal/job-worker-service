# jobhelper/Makefile

.PHONY: all deps build clean

HELPER_OUT = jobhelper

all: deps build

deps:
	@echo "Fetching dependencies for jobhelper..."
	go mod tidy

build: $(HELPER_OUT)

$(HELPER_OUT): jobhelper.go
	@echo "Building jobhelper..."
	go build -o $(HELPER_OUT) jobhelper.go

clean:
	@echo "Cleaning jobhelper build artifacts..."
	rm -f $(HELPER_OUT)

