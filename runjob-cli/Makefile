# Variables
OUT_DIR = .
GO_OUT_DIR = ./runproc-cli
MAIN = ./client.go
BINARY = runjob-cli

# Commands
.PHONY: all deps build clean run

all: deps build

# Fetch dependencies
deps:
	@echo "Fetching dependencies for runjob-cli..."
	go mod tidy

# Build the binary
build:
	go build -o $(BINARY) $(MAIN)

# Clean generated files and binary
clean:
	rm -f $(BINARY)

# Run the CLI (example: run a command)
run:
	./$(BINARY) start /bin/echo Hello, World!

