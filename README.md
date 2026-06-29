# ToolCapsule

**Portable, replayable WebAssembly tool capsules for MCP agents.**

ToolCapsule is an open-source runtime idea for making AI agent tools safer, more deterministic, and easier to debug. The core concept is simple: instead of letting an agent call arbitrary Python, Node.js, shell, or API code directly, small tools are packaged as WebAssembly capsules with strict schemas, resource limits, permissions, and replay logs.

This project is designed to be complementary to Blaxel, not a clone of Blaxel. Blaxel provides the fast sandbox/runtime infrastructure for agents. ToolCapsule provides a portable tool execution layer that can run locally with WebAssembly and optionally replay risky or failed executions inside Blaxel sandboxes.

## One-Line Pitch

ToolCapsule is a WASM runtime for MCP tools: schema-safe, resource-limited, replayable tool execution locally, with risky or failed runs replayed inside Blaxel sandboxes.

## Current Local Workflow

ToolCapsule now provides a local end-to-end workflow for Go tools:

```bash
go mod tidy
go run ./cmd/toolcapsule doctor
go run ./cmd/toolcapsule init /tmp/my_tool --lang go
go run ./cmd/toolcapsule dev /tmp/my_tool --input /tmp/my_tool/examples/input.json
go run ./cmd/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
go run ./cmd/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
go run ./cmd/toolcapsule replay runs/20260629.jsonl
go run ./cmd/toolcapsule report runs/20260629.jsonl --html --out report.html
go run ./cmd/toolcapsule bundle runs/20260629.jsonl --out run.tcbundle
go run ./cmd/toolcapsule replay run.tcbundle
go run ./cmd/toolcapsule cache list
```

The first `run` analyzes the tool, builds it as a WASI WebAssembly module, stores it under `.toolcapsule/cache`, runs it with `wazero`, and writes a JSONL run log under `runs/`.

The second `run` should report `"cache_hit": true` and execute the cached `tool.wasm` instead of rebuilding.

Available demo tools:

```bash
go run ./cmd/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/pii.json
go run ./cmd/toolcapsule run ./examples/tools/parse_csv --input examples/inputs/csv.json
go run ./cmd/toolcapsule run ./examples/tools/validate_json --input examples/inputs/validate_json.json
```

Failure and fallback demos:

```bash
go run ./cmd/toolcapsule run ./examples/tools/redact_pii --input examples/inputs/invalid_schema.json
go run ./cmd/toolcapsule run ./examples/tools/timeout_demo --input examples/inputs/empty.json
go run ./cmd/toolcapsule run ./examples/tools/bad_output_schema_demo --input examples/inputs/empty.json
go run ./cmd/toolcapsule analyze ./examples/tools/unsupported_network_tool
go run ./cmd/toolcapsule run ./examples/tools/unsupported_network_tool --input examples/inputs/empty.json --fallback docker
```

MCP mode exposes discovered manifests as MCP tools:

```bash
go run ./cmd/toolcapsule mcp serve ./examples/tools
go run ./cmd/toolcapsule mcp print-config ./examples/tools
go run ./cmd/toolcapsule mcp install claude ./examples/tools
```

Cache entries can be inspected or removed:

```bash
go run ./cmd/toolcapsule cache inspect <source_hash>
go run ./cmd/toolcapsule cache clean
```

## Problem

Modern AI agents increasingly use tools through MCP, function calling, or custom tool APIs. This is powerful, but production tool execution has several problems:

- Tools often run as unrestricted Python, Node.js, shell scripts, or backend services.
- LLMs can send malformed tool arguments.
- Tool output can violate the expected schema.
- Debugging failed tool calls is hard because the exact input, environment, stdout, stderr, and file changes are often not captured.
- Tool calls can hang, consume too much memory, or accidentally access resources they should not access.
- Local debugging and production execution often behave differently.
- Teams need a way to replay failed agent runs deterministically.

ToolCapsule focuses on this question:

> Can MCP tools be packaged like small, safe, deterministic execution units that are easy to validate, limit, record, and replay?

## Solution

ToolCapsule runs agent tools as WebAssembly modules. Each tool has a manifest that defines its name, input schema, output schema, timeout, memory limit, filesystem permissions, and network permissions.

Example tool manifest:

```yaml
name: redact_pii
runtime: wasm
module: tools/redact_pii.wasm
input_schema: schemas/redact_pii.input.json
output_schema: schemas/redact_pii.output.json
limits:
  timeout_ms: 300
  memory_mb: 32
permissions:
  network: false
  filesystem: read-only
```

An agent tool call comes in as JSON:

```json
{
  "tool": "redact_pii",
  "input": {
    "text": "Email me at alice@example.com or call +1 555 123 4567"
  }
}
```

ToolCapsule then performs the following steps:

1. Validate the tool name exists.
2. Validate the input against the tool input schema.
3. Run the tool inside a WASM runtime with timeout and memory limits.
4. Capture stdout, stderr, duration, exit status, and runtime errors.
5. Validate the output against the output schema.
6. Write a replayable run log.
7. Return a structured response to the agent.

Example response:

```json
{
  "ok": true,
  "tool": "redact_pii",
  "duration_ms": 18,
  "output": {
    "text": "Email me at [EMAIL] or call [PHONE]",
    "redactions": ["email", "phone"]
  }
}
```

## Why WebAssembly

WebAssembly is useful here because it gives agent tools a small, portable, sandboxed execution format.

Key benefits:

- Portable across local machines, CI, and cloud sandboxes.
- Faster startup than launching a full language runtime for small tools.
- Easier to restrict than arbitrary shell/Python/Node execution.
- Deterministic enough for many validation, parsing, transformation, and policy tools.
- Can be embedded in a Go runtime with libraries like `wazero`.
- Fits well with MCP tools that should behave like bounded functions.

This does not replace full sandboxes. WASM is good for small deterministic tools. Full OS-level sandboxes are still better for tasks that need package installation, browsers, shell access, long-running processes, or complex networking.

## Blaxel Connection

Blaxel provides infrastructure for autonomous agents: sandboxes, MCP server hosting, agent runtime, model gateway, batch jobs, and observability. ToolCapsule should use Blaxel as the cloud backend for executions that need stronger isolation or full OS capabilities.

The positioning is:

- Small deterministic tools run locally in WASM.
- Risky, failed, or long-running tool calls can be replayed inside Blaxel sandboxes.
- Blaxel is the secure runtime backend.
- ToolCapsule is the tool packaging, validation, and replay layer.

This avoids copying Blaxel. Instead, it demonstrates understanding of the agent infrastructure layer by building something useful on top of it.

Possible Blaxel integration modes:

- `local`: run the WASM tool directly with `wazero`.
- `blaxel-replay`: replay a failed run inside a Blaxel sandbox.
- `blaxel-escalate`: if a tool exceeds local WASM limits or requires full OS access, execute the fallback command inside a Blaxel sandbox.
- `blaxel-mcp`: expose ToolCapsule as an MCP server hosted on Blaxel.

## What Makes It Different

This is not another agent framework, MCP server list, or sandbox clone.

The unique angle is:

> Agent tools should be packaged as safe, replayable capsules with explicit contracts.

Most projects focus on giving agents more tools. ToolCapsule focuses on making tool execution safer and easier to debug.

The project combines:

- MCP tool calling
- WebAssembly execution
- JSON Schema validation
- Resource limits
- Replay logs
- Optional Blaxel sandbox replay
- Agent production debugging

## Example Tools

Good first tools for the project:

- `redact_pii`: redact emails, phone numbers, API keys, and common secrets from text.
- `parse_csv`: parse a CSV file or string and return row count, column names, and basic stats.
- `validate_json`: validate arbitrary JSON against a schema.
- `transform_json`: transform JSON using a safe mapping rule.
- `markdown_sanitize`: remove unsafe HTML or scripts from markdown.
- `safe_math`: evaluate bounded math expressions without arbitrary code execution.
- `diff_apply`: validate and apply a small patch safely.
- `token_budget_check`: check prompt size, token budget, and estimated cost.
- `policy_check`: check whether a proposed tool call violates a simple policy file.
- `sqlite_query_readonly`: run read-only SQLite queries against a mounted database file.

The best MVP tools are `redact_pii`, `parse_csv`, and `validate_json` because they are simple, useful, and easy to demo.

## MVP Scope

The first version should be small and impressive, not a large platform.

MVP features:

- Go CLI called `toolcapsule`.
- WASM execution with `wazero`.
- Tool manifest loading from YAML.
- Input JSON Schema validation.
- Output JSON Schema validation.
- Timeout limit.
- Memory limit if supported by the runtime configuration.
- Structured JSON response.
- Run log saved as JSONL.
- Replay command.
- Three demo tools: `redact_pii`, `parse_csv`, `validate_json`.
- Markdown report for failed runs.
- Optional Blaxel replay backend.

MVP commands:

```bash
toolcapsule run tools/redact_pii.yaml --input examples/pii.json
toolcapsule replay runs/failed-run.jsonl
toolcapsule report runs/failed-run.jsonl --out report.md
toolcapsule replay runs/failed-run.jsonl --backend blaxel
```

Example success output:

```json
{
  "ok": true,
  "tool": "redact_pii",
  "duration_ms": 21,
  "schema_valid": true,
  "output": {
    "text": "Contact [EMAIL] for access.",
    "redactions": ["email"]
  }
}
```

Example failure output:

```json
{
  "ok": false,
  "tool": "parse_csv",
  "duration_ms": 4,
  "error_type": "input_schema_validation_failed",
  "error": "missing required field: csv",
  "replay_log": "runs/2026-06-29T12-30-00Z.jsonl"
}
```

## Replay Log Format

Each tool call should produce a JSONL record. JSONL is simple, append-only, and easy to inspect.

Example record:

```json
{"run_id":"run_123","tool":"redact_pii","started_at":"2026-06-29T12:30:00Z","input":{"text":"alice@example.com"},"input_schema_valid":true,"duration_ms":18,"exit_code":0,"stdout":"","stderr":"","output":{"text":"[EMAIL]","redactions":["email"]},"output_schema_valid":true,"ok":true}
```

For failed runs, the log should capture enough information to reproduce the failure:

- Tool name
- Manifest version or hash
- WASM module hash
- Input JSON
- Input schema validation result
- Output JSON if any
- Output schema validation result
- stdout
- stderr
- exit code
- timeout status
- duration
- backend used: `local`, `blaxel-replay`, or `blaxel-escalate`

## Architecture

High-level flow:

```text
Agent or CLI
    |
    v
ToolCapsule CLI / MCP Server
    |
    v
Manifest Loader
    |
    v
Input Schema Validator
    |
    v
Execution Backend
    |-- Local WASM runtime with wazero
    |-- Optional Blaxel sandbox replay backend
    |
    v
Output Schema Validator
    |
    v
Run Log + Report
```

Core components:

- `cmd/toolcapsule`: CLI entry point.
- `internal/manifest`: load and validate tool manifests.
- `internal/schema`: input and output JSON Schema validation.
- `internal/runtime/wasm`: local WASM execution using `wazero`.
- `internal/runtime/blaxel`: optional Blaxel sandbox replay backend.
- `internal/recorder`: JSONL run log writer.
- `internal/replay`: replay failed or successful runs.
- `internal/report`: Markdown or HTML timeline report generation.
- `examples/tools`: demo tool capsules.

## Suggested Repository Structure

```text
toolcapsule/
  README.md
  go.mod
  cmd/
    toolcapsule/
      main.go
  internal/
    manifest/
    schema/
    runtime/
      wasm/
      blaxel/
    recorder/
    replay/
    report/
  examples/
    tools/
      redact_pii/
      parse_csv/
      validate_json/
    inputs/
    runs/
  schemas/
  docs/
    demo.md
    blaxel-integration.md
```

## MCP Mode

After the CLI works, ToolCapsule can expose an MCP server.

In MCP mode, an agent can call capsule tools through MCP:

```text
Agent -> MCP Client -> ToolCapsule MCP Server -> WASM Capsule -> Structured Output
```

The MCP server should expose tools based on available manifests. If `tools/redact_pii.yaml` exists, the MCP server exposes `redact_pii` as a tool with the input schema from the manifest.

This makes the project directly relevant to Blaxel because Blaxel supports hosted MCP servers and sandbox-native MCP workflows.

## Blaxel Replay Backend

The Blaxel backend can start simple.

Version 1 can do this:

1. Read a failed JSONL run.
2. Create or reuse a Blaxel sandbox.
3. Upload the tool manifest, input JSON, and runner script.
4. Execute the replay command inside the sandbox.
5. Fetch stdout, stderr, and result JSON.
6. Append a new replay record to the run log.

The goal is not to replace local WASM execution. The goal is to prove that failed or risky agent tool calls can be reproduced inside a secure cloud sandbox.

Example command:

```bash
toolcapsule replay runs/failed-run.jsonl --backend blaxel --sandbox-name toolcapsule-debug
```

## Demo Plan

The demo should be visual and easy to understand.

Demo title:

**From malformed MCP call to replayable WASM capsule**

Demo flow:

1. Run `redact_pii` with valid input.
2. Show successful schema validation and fast WASM execution.
3. Run `parse_csv` with malformed input.
4. Show input schema failure before execution.
5. Run a tool that intentionally times out.
6. Show timeout enforcement and replay log.
7. Replay the failed run locally.
8. Replay the failed run inside Blaxel sandbox.
9. Generate a Markdown report.

Example final report:

```text
Run ID: run_123
Tool: parse_csv
Status: failed
Failure: input_schema_validation_failed
Duration: 4ms
Backend: local-wasm
Replay: successful inside Blaxel sandbox
```

## What To Show In The README

The public README should make the project look sharp within the first 30 seconds.

Important sections:

- Short GIF or terminal demo.
- One-line pitch.
- Why agent tools need capsules.
- Quickstart.
- Example manifest.
- Example failed run and replay.
- Blaxel integration section.
- Roadmap.

The most important positioning line:

> ToolCapsule is not a sandbox platform. It is a portable safety and replay layer for MCP tools, designed to run locally with WASM and replay risky executions inside Blaxel sandboxes.

## Roadmap

Phase 1: Local CLI

- Create Go CLI.
- Implement manifest loading.
- Implement JSON Schema validation.
- Run WASM modules with timeout.
- Save JSONL run logs.
- Add replay command.

Phase 2: Demo Capsules

- Add `redact_pii`.
- Add `parse_csv`.
- Add `validate_json`.
- Add failure examples.
- Add Markdown report generation.

Phase 3: MCP Server

- Expose capsule tools through MCP.
- Generate MCP tool schemas from manifests.
- Record every MCP tool call.
- Support replay from MCP traces.

Phase 4: Blaxel Integration

- Add Blaxel SDK integration.
- Replay failed runs inside Blaxel sandboxes.
- Add optional Blaxel deployment guide.
- Add demo video using Blaxel sandbox backend.

Phase 5: Policy And Security

- Add policy files for permissions.
- Add secret redaction in logs.
- Add module signing or hashing.
- Add allowlist for approved capsules.

## Why This Fits Blaxel

This project demonstrates the exact type of thinking Blaxel likely cares about:

- Agent runtime design
- Sandboxed execution
- MCP tools
- Go-based infrastructure
- Safety and reliability
- Debuggability
- Observability
- Replayable production failures
- Clear understanding of local-vs-cloud execution tradeoffs

It also shows product judgment. Instead of rebuilding Blaxel, the project builds a useful layer that could make Blaxel-hosted agents easier to debug and trust.

## Outreach Angle

Potential message to Blaxel:

> I started building ToolCapsule, an OSS WASM runtime for MCP tools: schema-safe, resource-limited, replayable tool execution locally, with failed or risky runs replayed inside Blaxel sandboxes. I wanted to build something complementary to Blaxel rather than cloning the infra layer. The goal is to make agent tool calls easier to validate, debug, and replay in production.

## Success Criteria

The project is successful if a viewer can understand the value in under one minute:

- Agents call tools.
- Tool calls fail in weird ways.
- ToolCapsule packages tools as safe WASM capsules.
- Every call is validated, bounded, recorded, and replayable.
- Blaxel is used as the secure cloud replay backend.

## Initial Build Recommendation

Start with this order:

1. Build the CLI with a fake runtime that only validates schemas and writes logs.
2. Add `wazero` execution for one simple WASM tool.
3. Add `redact_pii` as the first demo capsule.
4. Add failure logging and replay.
5. Add `parse_csv` and `validate_json`.
6. Add Markdown report generation.
7. Add Blaxel replay as the final differentiator.

Do not start with the Blaxel integration first. The local WASM + replay demo should work independently. Blaxel should be the impressive cloud backend layer after the core idea is already clear.
