package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeRedactPII(t *testing.T) {
	result, err := Analyze("../../examples/tools/redact_pii")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Capsulable {
		t.Fatalf("expected redact_pii to be capsulable: %#v", result)
	}
	if result.Language != "go" {
		t.Fatalf("expected go language, got %q", result.Language)
	}
}

func TestAnalyzeBlocksRiskyImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module risky\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(dir, "toolcapsule.yaml"), `name: risky
language: go
input_schema: input.schema.json
output_schema: output.schema.json
limits:
  timeout_ms: 1000
  memory_mb: 32
permissions:
  network: false
  filesystem: none
build:
  target: wasip1
`)
	mustWrite(t, filepath.Join(dir, "input.schema.json"), `{"type":"object"}`)
	mustWrite(t, filepath.Join(dir, "output.schema.json"), `{"type":"object"}`)
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\nimport _ \"os/exec\"\nfunc main(){}\n")
	result, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Capsulable {
		t.Fatalf("expected risky tool to be blocked: %#v", result)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
