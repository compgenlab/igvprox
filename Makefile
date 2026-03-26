BINARY := igvprox
BIN_DIR := bin
PKG := ./cmd/igvprox

.PHONY: all build build-linux-amd64 build-linux-arm64 clean test fmt

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

build-linux-amd64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY)-linux-amd64 $(PKG)

build-linux-arm64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(PKG)

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

clean:
	rm -rf $(BIN_DIR)
