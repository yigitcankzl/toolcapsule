# Contributing to ToolCapsule

Thanks for your interest — issues and PRs are welcome.

## Development

Requirements: Go 1.22+ (Docker only if you use the `--fallback docker` path).

```bash
make build      # build ./bin/toolcapsule
make test       # go test ./...
make demo       # build + run the demo
./bin/toolcapsule doctor   # check your toolchain
```

If Go isn't on `PATH`, point ToolCapsule at it: `TOOLCAPSULE_GO=/path/to/go`.

## Pull requests

- Keep PRs focused and small where possible.
- For a new feature, open an issue first so we can agree on the shape.
- Run `make test` and `make demo` before pushing.
- Match the existing style: standard `gofmt`, small packages under `internal/`, structured JSON output for commands.

## Adding an example tool

Each tool lives under `examples/tools/<name>/` with:

- a `toolcapsule.yaml` manifest (name, language, input/output schema, limits, permissions),
- `input.schema.json` / `output.schema.json`,
- the tool source (e.g. a Go program compiled to a WASI capsule).

See `examples/tools/parse_csv` for a minimal reference, then run it with
`./bin/toolcapsule run ./examples/tools/<name> --input examples/inputs/<name>.json`.

## Releases

Pushing a `v*` tag triggers CI: it cross-compiles the binaries, publishes a
GitHub Release with checksums, and pushes a multi-arch Docker image to GHCR.
Build the same artifacts locally with `make release` and `make docker`.
