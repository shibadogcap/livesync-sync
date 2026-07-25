.PHONY: all build build-server win64 linux docker clean test

APP := livesync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -s -w"

# Default: build with tray support (requires CGO)
all: build

# Linux desktop build (CGO enabled for tray support)
build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o build/$(APP) ./cmd/$(APP)

# Server-only build (no tray, CGO disabled, works in Docker)
build-server:
	CGO_ENABLED=0 go build $(LDFLAGS) -tags notray -o build/$(APP)-server ./cmd/$(APP)

# Windows cross-compile (requires mingw-w64)
win64:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
	CC=x86_64-w64-mingw32-gcc \
	go build $(LDFLAGS) -ldflags "-H=windowsgui -X main.version=$(VERSION) -s -w" \
	-o build/$(APP).exe ./cmd/$(APP)

# Linux (no tray, stripped)
linux: build-server
	@echo "Linux server build: build/$(APP)-server"

# Docker image
docker:
	docker build -t livesync-sync:$(VERSION) .

# Quick build for development
dev:
	go build -o build/$(APP)-dev ./cmd/$(APP)

# Clean build artifacts
clean:
	rm -rf build/

# Update dependencies
deps:
	go mod tidy

# Run tests
test:
	go test ./...

# Strip debug info for smaller binary
strip: build-server
	strip build/$(APP)-server
	@echo "Stripped size:"
	@ls -lh build/$(APP)-server
