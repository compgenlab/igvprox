BINARY := igvprox
BIN_DIR := bin
PKG := ./cmd/igvprox

.PHONY: all build clean test fmt

all: build

build: $(BIN_DIR)/$(BINARY).darwin_arm64 $(BIN_DIR)/$(BINARY).linux_amd64 $(BIN_DIR)/$(BINARY).linux_arm64

$(BIN_DIR)/$(BINARY).darwin_arm64:
	mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o $@ $(PKG)

$(BIN_DIR)/$(BINARY).linux_amd64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $@ $(PKG)

$(BIN_DIR)/$(BINARY).linux_arm64:
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $@ $(PKG)

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

clean:
	rm -rf $(BIN_DIR)
