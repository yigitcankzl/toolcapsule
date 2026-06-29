BINARY := toolcapsule
GO ?= go
PREFIX ?= $(HOME)/.local

.PHONY: build test demo install clean

build:
	mkdir -p bin
	$(GO) build -o bin/$(BINARY) ./cmd/toolcapsule

test:
	$(GO) test ./...

demo: build
	./bin/$(BINARY) demo

install: build
	mkdir -p $(PREFIX)/bin
	cp bin/$(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "installed $(PREFIX)/bin/$(BINARY)"

clean:
	rm -rf bin
