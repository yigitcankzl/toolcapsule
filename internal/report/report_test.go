package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.jsonl")
	data := `{"run_id":"run_1","started_at":"2026-06-29T12:00:00Z","ok":true,"tool":"demo","backend":"local-wasm","cache_hit":true,"source_hash":"sha256_x","duration_ms":12,"input":{"text":"hello"},"input_schema_valid":true,"output":{"text":"hello"},"output_schema_valid":true,"stdout":"{}","stderr":""}` + "\n"
	if err := os.WriteFile(logPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	markdown, result, err := Generate(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Records != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(markdown, "ToolCapsule Run Report") {
		t.Fatal("expected report title")
	}
	if !strings.Contains(markdown, "run_1") {
		t.Fatal("expected run id")
	}
}
