# ToolCapsule

**A runtime and gateway for safer AI agent tools.**

ToolCapsule helps teams expose internal scripts, data lookups, security utilities, and automation tools to AI agents without giving the agent raw shell, database, or arbitrary code access.

The core idea is simple: wrap every tool with a manifest, JSON Schema contracts, sandboxed execution, replayable logs, reports, and MCP/HTTP interfaces.

```text
Claude Code / MCP client / backend / CI
        |
        v
ToolCapsule gateway
        |
        +-- validate input schema
        +-- run a WASM capsule or Docker fallback
        +-- validate output schema
        +-- write replay logs and reports
        v
Internal tools and data utilities
```

Build a tool once. Run it as CLI, MCP, HTTP, automation, or a signed plugin.

## Common Industry Use Cases

### Internal MCP Tool Gateway

Most teams already have useful scripts and small internal tools:

```text
query_customer
search_logs
create_ticket
redact_pii
validate_json
parse_csv
run_policy_check
```

Exposing those directly to Claude Code, Cursor, Cline, OpenCode, or another agent is risky. ToolCapsule sits between the agent and the tools:

```text
Agent -> MCP -> ToolCapsule -> validated, sandboxed, replayable tool execution
```

The agent sees normal MCP tools. The platform team gets schemas, sandboxing, logs, replay, and reports.

### Controlled Internal Data Access

Instead of giving an agent raw SQL, shell access, or broad API credentials, expose narrow read-only tools.

Example request:

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

ToolCapsule validates the shape, runs the tool in a bounded backend, validates the output, and records the call. The agent can answer questions over internal data without getting arbitrary database access.

### Agent Tool Debugging

When an agent tool fails, teams need to know:

- What exact input did the agent send?
- Did the input pass schema validation?
- Did the tool timeout or fail during execution?
- What stdout, stderr, duration, hashes, and backend were involved?
- Can the call be replayed?

ToolCapsule records each call as JSONL and can replay or report it:

```bash
./bin/toolcapsule replay
./bin/toolcapsule report --html --out report.html
./bin/toolcapsule dashboard
```

### Signed Internal Tool Plugins

Teams can package tools as signed capsules and distribute them internally.

```bash
./bin/toolcapsule keygen --out team.key --pub-out team.pub
./bin/toolcapsule run ./examples/tools/parse_csv --input examples/inputs/csv.json
./bin/toolcapsule bundle --out parse_csv.tcbundle --sign --key team.key
./bin/toolcapsule verify parse_csv.tcbundle --pubkey team.pub
./bin/toolcapsule plugin install parse_csv.tcbundle --pubkey team.pub
./bin/toolcapsule run parse_csv --input examples/inputs/csv.json
```

This creates a lightweight supply-chain story for agent tools: signed artifact, verified install, run by name.

### CI Contract Testing

Agent tools should be tested like normal software artifacts. ToolCapsule can run tool contract checks in CI or automation:

```bash
make build
./bin/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
./bin/toolcapsule replay
./bin/toolcapsule report --html --out tool-report.html
```

This catches schema, output, timeout, and runtime regressions before an agent depends on the tool.

## Why ToolCapsule

ToolCapsule is useful when you want agent tools to be:

- **Schema-validated**: inputs and outputs are checked with JSON Schema.
- **Sandboxed**: suitable tools run as WASI WebAssembly through `wazero`; non-WASM tools can use Docker fallback.
- **Replayable**: every call becomes a JSONL record that can be replayed.
- **Auditable**: reports capture stdout, stderr, duration, backend, cache status, and hashes.
- **Portable**: the same tool can run as CLI, MCP, HTTP, CI, or a signed plugin.
- **Fast when warm**: persisted `wazero` compilation cache lets warm capsules start in tens of milliseconds.

```text
Not just an MCP server.
Not just a WASM runner.
ToolCapsule is the contract, runtime, gateway, and replay layer around agent tools.
```

## Quick Demo

Build from source:

```bash
git clone https://github.com/yigitcankzl/toolcapsule.git
cd toolcapsule
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

If Go is not on `PATH`, point ToolCapsule at it:

```bash
GO=/path/to/go make build
TOOLCAPSULE_GO=/path/to/go ./bin/toolcapsule demo
```

Requirements:

- Go 1.22+
- Docker, only if using `--fallback docker`

## Try With Claude Code

Build the binary:

```bash
make build
```

Add ToolCapsule as a Claude Code MCP server:

```bash
claude mcp add toolcapsule -- "$PWD/bin/toolcapsule" mcp serve "$PWD/examples/tools"
```

If Claude Code cannot find Go while building capsules, pass the Go path into the MCP subprocess:

```bash
claude mcp remove toolcapsule
claude mcp add toolcapsule \
  -e TOOLCAPSULE_GO=/path/to/go \
  -- "$PWD/bin/toolcapsule" mcp serve "$PWD/examples/tools"
```

Check the server:

```bash
claude mcp list
```

Prompt to test:

```text
Call the ToolCapsule query_table_readonly tool with this input:

{
  "table": "tickets",
  "select": ["id", "customer_id", "severity", "status"],
  "where": {
    "severity": "high",
    "status": "open"
  },
  "limit": 5
}

Return the rows and explain whether the tool call was schema-validated and sandboxed.
```

Expected rows:

```json
[
  {
    "id": "tic_101",
    "customer_id": "cus_001",
    "severity": "high",
    "status": "open"
  },
  {
    "id": "tic_103",
    "customer_id": "cus_001",
    "severity": "high",
    "status": "open"
  }
]
```

## Core Workflows

### CLI

Run tools directly:

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

### Signed Plugins

Create a signing key:

```bash
./bin/toolcapsule keygen --out /tmp/toolcapsule.key --pub-out /tmp/toolcapsule.pub
```

Build a run record, export a signed bundle, and verify it:

```bash
./bin/toolcapsule run ./examples/tools/parse_csv --input examples/inputs/csv.json
./bin/toolcapsule bundle --out /tmp/parse_csv.tcbundle --sign --key /tmp/toolcapsule.key
./bin/toolcapsule verify /tmp/parse_csv.tcbundle --pubkey /tmp/toolcapsule.pub
```

Install and run the bundle as a plugin:

```bash
./bin/toolcapsule plugin install /tmp/parse_csv.tcbundle --pubkey /tmp/toolcapsule.pub
./bin/toolcapsule plugin list
./bin/toolcapsule run parse_csv --input examples/inputs/csv.json
```

Plugins install under `~/.toolcapsule/plugins` by default. Set `TOOLCAPSULE_HOME` to use a different plugin home in tests or sandboxes.

## Realistic Example: Read-Only Data Query

`query_table_readonly` shows a common enterprise pattern: answer bounded questions over internal data without giving the model arbitrary database or shell access.

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

This is intentionally narrower than raw SQL. The tool exposes a controlled read-only query shape, validates the arguments, runs inside the same runtime, and returns schema-checked rows. The same pattern can later wrap SQLite, warehouses, or internal APIs behind Docker/server backends while keeping the ToolCapsule contract stable.

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
3. Analyzes source for obvious unsupported patterns when building from source.
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

### Cold Start

WebAssembly is chosen for isolation and startup cost. A warm capsule executes in tens of milliseconds; the equivalent OS-level sandbox is orders of magnitude slower.

| Backend | Per-call latency | Notes |
|---|---|---|
| `local-wasm`, first run | ~740 ms | wazero compiles the Go wasm module |
| `local-wasm`, warm compilation cache | ~53 ms | compiled module persisted per capsule |
| `docker-sandbox` | ~27 s | builds and runs the Go tool in a container |

Measured locally on the `parse_csv` example. The warm number comes from a persistent `wazero` compilation cache stored beside each capsule, so the module is compiled once and reused across processes. TinyGo capsules are smaller again and start faster still.

Optional toolchain environment variables:

```bash
TOOLCAPSULE_GO=/path/to/go
TOOLCAPSULE_TINYGO=/path/to/tinygo
TOOLCAPSULE_CARGO=/path/to/cargo
TOOLCAPSULE_JAVY=/path/to/javy
TOOLCAPSULE_DOCKER_GO_IMAGE=golang:1.22
```

## Why Not Just Write An MCP Server?

You can. ToolCapsule is useful when you want the runtime concerns handled consistently:

- One tool can run as CLI, MCP, HTTP, automation, or a signed plugin.
- Input/output contracts are enforced with JSON Schema.
- Every call is logged and replayable.
- Reports help debug agent/tool failures.
- WASM and Docker backends provide a safer execution boundary than direct shelling out.
- Tool contracts can be tested before being exposed to an agent.
- Signed bundles make internal distribution easier to audit.

## Status

ToolCapsule is an early working prototype. It is useful for demos, experimentation, and local/internal tool workflows, but the API and manifest format may still change.

Near-term roadmap:

- Release binaries.
- Docker image.
- Homebrew install path.
- Dedicated GitHub Action.
- More realistic tool templates.
- Stronger permission policy.
- Signed bundle provenance improvements.
- Optional remote sandbox backends.
