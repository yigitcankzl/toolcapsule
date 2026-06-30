BINARY := toolcapsule
GO ?= go
PREFIX ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
IMAGE ?= ghcr.io/yigitcankzl/toolcapsule
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build test demo install clean release docker docker-run

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/toolcapsule

test:
	$(GO) test ./...

demo: build
	./bin/$(BINARY) demo

install: build
	mkdir -p $(PREFIX)/bin
	cp bin/$(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "installed $(PREFIX)/bin/$(BINARY)"

# Cross-compiled release binaries -> dist/
release:
	@rm -rf dist
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  out=dist/$(BINARY)-$$os-$$arch; \
	  echo "building $$out"; \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/toolcapsule || exit 1; \
	done
	@echo "---"; ls -la dist

# Build the Docker image (keeps the Go toolchain so capsules build at runtime)
docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

# Run the demo in the container — zero host setup
docker-run:
	docker run --rm $(IMAGE):latest demo

clean:
	rm -rf bin dist
