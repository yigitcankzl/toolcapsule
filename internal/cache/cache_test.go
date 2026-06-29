package cache

import (
	"os"
	"path/filepath"
	"testing"

	"toolcapsule/internal/manifest"
)

func TestSaveIsDiscoverable(t *testing.T) {
	projectRoot := t.TempDir()
	toolDir := t.TempDir()
	wasmPath := filepath.Join(t.TempDir(), "tool.wasm")
	mustWrite(t, wasmPath, []byte("wasm"))
	mustWrite(t, filepath.Join(toolDir, "input.schema.json"), []byte(`{"type":"object"}`))
	mustWrite(t, filepath.Join(toolDir, "output.schema.json"), []byte(`{"type":"object"}`))

	m := manifest.Manifest{
		Name:         "demo",
		Language:     "go",
		InputSchema:  "input.schema.json",
		OutputSchema: "output.schema.json",
		Limits:       manifest.Limits{TimeoutMS: 1000, MemoryMB: 32},
		Permissions:  manifest.Permissions{Network: false, Filesystem: "none"},
	}
	entry, err := Save(projectRoot, toolDir, "sha256_test", wasmPath, m)
	if err != nil {
		t.Fatal(err)
	}
	if !Exists(projectRoot, "sha256_test") {
		t.Fatal("expected saved cache entry to exist")
	}
	if _, err := os.Stat(filepath.Join(entry.Path, "tool.wasm")); err != nil {
		t.Fatal(err)
	}
	entries, err := List(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 cache entry, got %d", len(entries))
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
