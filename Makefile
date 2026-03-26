BINARY := igvprox
BIN_DIR := bin
PKG := ./cmd/igvprox

.PHONY: all build clean test fmt

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

clean:
	rm -rf $(BIN_DIR)
