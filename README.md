# ToolCapsule

**Portable, local-first runtime for AI agent tools.**

Build tools once. Run them as CLI, MCP, HTTP, or automation jobs with JSON Schema validation, sandboxed execution, replayable logs, and reports.

```text
tool source + manifest
        |
        v
  ToolCapsule runtime
        |
        +-- CLI
        +-- MCP server
        +-- HTTP server
        +-- CI / automation
```

ToolCapsule is local-first, not local-only. It can run on a developer machine, inside CI, in a Docker container, or as an internal HTTP tool gateway. The first execution backend is WASI WebAssembly through `wazero`; tools that are not a good WASM fit can use the Docker fallback path behind the same validation, logging, replay, and reporting layer.

## Why

AI agents increasingly call tools through MCP, function calling, or custom APIs. That is useful, but raw tool execution is hard to trust and debug.

ToolCapsule gives each tool a contract and execution envelope:

- Validate tool input before execution.
- Run suitable tools in a WASM sandbox with timeout and memory limits.
- Validate tool output before returning it to the agent.
- Record every call as replayable JSONL.
- Generate HTML reports for debugging.
- Expose the same tool through CLI, MCP, or HTTP.
- Use Docker fallback for tools that need a fuller OS runtime.

The short version:

```text
Not just an MCP server.
Not just a WASM runner.
ToolCapsule is the contract, runtime, gateway, and replay layer around agent tools.
```

## 60-Second Demo

Build from source:

```bash
make build
./bin/toolcapsule doctor
./bin/toolcapsule demo
```

The demo runs successful tools, expected schema failures, output validation failures, report generation, and bundle export. It writes replayable logs under `runs/`.

Replay and inspect the latest run:

```bash
./bin/toolcapsule replay
./bin/toolcapsule report --html --out report.html
./bin/toolcapsule dashboard
```

## Install From Source

Requirements:

- Go 1.22+
- Docker, only if using `--fallback docker`

```bash
git clone https://github.com/yigitcankzl/toolcapsule.git
cd toolcapsule
make build
```

Optional local install:

```bash
./scripts/install-local.sh
toolcapsule doctor
```

If Go is not on `PATH`, point ToolCapsule at it:

```bash
TOOLCAPSULE_GO=/path/to/go make build
TOOLCAPSULE_GO=/path/to/go ./bin/toolcapsule demo
```

## Core Workflows

### CLI

Run a tool directly:

```bash
./bin/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
./bin/toolcapsule run ./examples/tools/parse_csv --input examples/inputs/csv.json
./bin/toolcapsule run ./examples/tools/query_table_readonly --input examples/inputs/query_table.json
```

The first run analyzes the tool, builds a WASI WebAssembly capsule when possible, caches it under `.toolcapsule/cache`, executes it, validates the output, and appends a JSONL record under `runs/`.

The second run of the same source should show `"cache_hit": true`.

### MCP

Expose all example tools as MCP tools:

```bash
./bin/toolcapsule mcp serve ./examples/tools
```

Print an MCP config snippet:

```bash
./bin/toolcapsule mcp print-config ./examples/tools
```

Install config for supported clients:

```bash
./bin/toolcapsule mcp install claude ./examples/tools
./bin/toolcapsule mcp install opencode ./examples/tools
```

### HTTP

Expose the same tools over HTTP:

```bash
./bin/toolcapsule serve --http 127.0.0.1:8080 ./examples/tools
```

Call a tool:

```bash
curl -X POST http://127.0.0.1:8080/v1/tools/parse_csv/call \
  -H 'Content-Type: application/json' \
  -d '{"input":{"csv":"a,b\n1,2"}}'
```

HTTP endpoints:

```text
GET  /healthz
GET  /readyz
GET  /v1
GET  /v1/tools
POST /v1/tools/{name}/call
POST /v1/tools/{name}/run
GET  /v1/runs/latest
```

Tool calls accept `{"input": {...}}`, `{"arguments": {...}}`, or the input object directly.

### Replay And Reports

ToolCapsule records every run as JSONL. `runs/latest.jsonl` points to the latest record, so replay and reporting work without passing a path:

```bash
./bin/toolcapsule replay
./bin/toolcapsule replay --latest-failed
./bin/toolcapsule report --html --out report.html
./bin/toolcapsule bundle --out run.tcbundle
./bin/toolcapsule replay run.tcbundle
```

### Automation Or CI

ToolCapsule can be used in automation even before a dedicated GitHub Action exists:

```bash
make build
./bin/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
./bin/toolcapsule replay
./bin/toolcapsule report --html --out tool-report.html
```

Use this to catch tool contract regressions before an agent depends on them.

## Realistic Example: Read-Only Data Query

`query_table_readonly` is a small example of the kind of tool agents often need: ask a bounded question over internal data without giving the model arbitrary database or shell access.

```bash
./bin/toolcapsule run ./examples/tools/query_table_readonly --input examples/inputs/query_table.json
```

Example input:

```json
{
  "table": "tickets",
  "select": ["id", "customer_id", "severity", "status"],
  "where": {
    "severity": "high",
    "status": "open"
  },
  "limit": 5
}
```

This is intentionally narrower than raw SQL. The tool exposes a controlled read-only query shape, validates the arguments, runs inside the same runtime, and returns schema-checked rows. The same pattern can later wrap SQLite or warehouse queries behind Docker/server backends while keeping the ToolCapsule contract stable.

## Tool Manifest

Each tool has a `toolcapsule.yaml` manifest:

```yaml
name: parse_csv
language: go
input_schema: input.schema.json
output_schema: output.schema.json
limits:
  timeout_ms: 3000
  memory_mb: 32
permissions:
  network: false
  filesystem: none
build:
  target: wasip1
```

The manifest makes execution explicit:

- Tool name.
- Build language and target.
- Input and output JSON Schemas.
- Timeout and memory limits.
- Network and filesystem policy.

## Example Tools

Included tools:

- `redact_pii`: redact emails, phone numbers, and common sensitive values.
- `parse_csv`: parse a CSV string and return row count plus columns.
- `validate_json`: validate a JSON value against a supplied schema.
- `query_table_readonly`: run a bounded read-only query over bundled table data.
- `timeout_demo`: demonstrate timeout handling.
- `bad_output_schema_demo`: demonstrate output schema failure.
- `unsupported_network_tool`: demonstrate analyzer rejection and Docker fallback.

Run them:

```bash
./bin/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
./bin/toolcapsule run ./examples/tools/parse_csv --input examples/inputs/csv.json
./bin/toolcapsule run ./examples/tools/validate_json --input examples/inputs/validate_json.json
./bin/toolcapsule run ./examples/tools/query_table_readonly --input examples/inputs/query_table.json
```

Failure and fallback demos:

```bash
./bin/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/invalid_schema.json
./bin/toolcapsule run ./examples/tools/timeout_demo --input examples/inputs/empty.json
./bin/toolcapsule run ./examples/tools/bad_output_schema_demo --input examples/inputs/empty.json
./bin/toolcapsule analyze ./examples/tools/unsupported_network_tool
./bin/toolcapsule run ./examples/tools/unsupported_network_tool --input examples/inputs/empty.json --fallback docker
```

## How It Works

For each call, ToolCapsule:

1. Loads the tool manifest.
2. Validates the input JSON against the input schema.
3. Analyzes the source for obvious unsupported patterns.
4. Builds or reuses a cached WASI WebAssembly capsule when possible.
5. Executes the tool with timeout and memory limits.
6. Captures stdout, stderr, duration, cache status, hashes, and errors.
7. Validates stdout JSON against the output schema.
8. Appends a replayable run record.
9. Returns a structured result to CLI, MCP, or HTTP callers.

## Execution Backends

Current backends:

- `local-wasm`: Go/TinyGo/Rust/Javy tools compiled to WASI WebAssembly when the matching toolchain is installed.
- `docker-sandbox`: hardened Docker fallback for non-capsulable Go tools when `--fallback docker` is used.

Current hardening:

- JSON Schema validation uses `github.com/santhosh-tekuri/jsonschema/v6`.
- WASM runs through `wazero` with timeout and memory page limits.
- Go analysis uses AST imports plus `go list -deps`.
- Docker fallback uses read-only source mounts, dropped capabilities, `no-new-privileges`, CPU/pid/memory limits, tmpfs build cache, and manifest-controlled network access.

Optional toolchain environment variables:

```bash
TOOLCAPSULE_GO=/path/to/go
TOOLCAPSULE_TINYGO=/path/to/tinygo
TOOLCAPSULE_CARGO=/path/to/cargo
TOOLCAPSULE_JAVY=/path/to/javy
TOOLCAPSULE_DOCKER_GO_IMAGE=golang:1.22
```

## Why Not Just Write An MCP Server?

You can. ToolCapsule is useful when you want the tool runtime concerns handled consistently:

- One tool can run as CLI, MCP, HTTP, or automation.
- Input/output contracts are enforced with JSON Schema.
- Every call is logged and replayable.
- Reports help debug agent/tool failures.
- WASM and Docker backends give a safer execution boundary than direct shelling out.
- Tool contracts can be tested before being exposed to an agent.

## Status

ToolCapsule is an early working prototype. It is useful for demos, experimentation, and local/internal tool workflows, but the API and manifest format may still change.

Near-term roadmap:

- Release binaries.
- Docker image.
- Homebrew install path.
- Dedicated GitHub Action.
- More realistic tool templates.
- Stronger permission policy.
- Signed bundles and provenance.
- Optional remote sandbox backends.
