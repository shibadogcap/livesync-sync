.PHONY: all build build-server win64 win-console win-gui linux docker clean test win-rsrc

APP := livesync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -s -w"

# Default: build Linux desktop + Windows console (primary)
all: build win-console

# Linux desktop build (CGO enabled for tray support)
build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o build/$(APP) ./cmd/$(APP)

# Server-only build (no tray, CGO disabled, works in Docker)
build-server:
	CGO_ENABLED=0 go build $(LDFLAGS) -tags notray -o build/$(APP)-server ./cmd/$(APP)

# Windows resource file (manifest + version info)
# Requires mingw-w64: apt install gcc-mingw-w64-x86-64
win-rsrc:
	cd cmd/$(APP) && x86_64-w64-mingw32-windres -o rsrc_windows.syso livesync.rc

# Windows console build (CGO enabled, may trigger Defender)
win-console: win-rsrc
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
	CC=x86_64-w64-mingw32-gcc \
	go build -ldflags "-X main.version=$(VERSION) -s -w" \
	-o build/$(APP)-console.exe ./cmd/$(APP)

# Windows CGO-disabled build (recommended, least Defender false positives)
# No tray icon, but settings UI available via browser.
win-nocgo:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
	go build -tags notray -ldflags "-X main.version=$(VERSION) -s -w" \
	-o build/$(APP)-nocgo.exe ./cmd/$(APP)

# Windows GUI build (tray only, may trigger Defender)
win-gui: win-rsrc
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
	CC=x86_64-w64-mingw32-gcc \
	go build -ldflags "-H=windowsgui -X main.version=$(VERSION) -s -w" \
	-o build/$(APP).exe ./cmd/$(APP)

# Windows all variants
win: win-nocgo win-console win-gui

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
