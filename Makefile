BINARY := igvprox
BIN_DIR := bin
PKG := ./cmd/igvprox

.PHONY: all build clean test fmt

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) $(PKG)

$(BIN_DIR)/$(BINARY)-darwin-arm64:
	mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $@ $(PKG)

$(BIN_DIR)/$(BINARY)-linux-amd64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $@ $(PKG)

$(BIN_DIR)/$(BINARY)-linux-arm64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $@ $(PKG)

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

clean:
	rm -rf $(BIN_DIR)
