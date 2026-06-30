# syntax=docker/dockerfile:1

# --- build stage ---------------------------------------------------------
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/toolcapsule ./cmd/toolcapsule

# --- runtime stage -------------------------------------------------------
# NOTE: ToolCapsule compiles tools to WASI WebAssembly at runtime, so the
# image keeps the Go toolchain (golang base). This is intentional, not bloat:
# it lets `docker run ... demo` build + run capsules with zero host setup.
FROM golang:1.24-bookworm
LABEL org.opencontainers.image.source="https://github.com/yigitcankzl/toolcapsule" \
      org.opencontainers.image.description="A runtime and gateway for safer AI agent tools" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY --from=build /out/toolcapsule /usr/local/bin/toolcapsule
COPY examples ./examples
# toolcapsule finds the toolchain via TOOLCAPSULE_GO or PATH; pin it explicitly.
ENV TOOLCAPSULE_GO=/usr/local/go/bin/go \
    TOOLCAPSULE_HOME=/app/.toolcapsule
EXPOSE 8080
ENTRYPOINT ["toolcapsule"]
CMD ["demo"]
