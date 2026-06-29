package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateObject(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	data := `{
		"type": "object",
		"required": ["text"],
		"additionalProperties": false,
		"properties": {
			"text": { "type": "string" }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(s, map[string]any{"text": "hello"}); err != nil {
		t.Fatalf("expected valid input: %v", err)
	}
	if err := Validate(s, map[string]any{}); err == nil {
		t.Fatal("expected missing required field error")
	}
	if err := Validate(s, map[string]any{"text": "hello", "extra": true}); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestValidateDraftFeatures(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	data := `{
		"type": "object",
		"required": ["kind", "name", "count"],
		"properties": {
			"kind": { "enum": ["demo"] },
			"name": { "type": "string", "pattern": "^[a-z]+$" },
			"count": { "type": "integer", "minimum": 1 }
		}
	}`
	if err := os.WriteFile(schemaPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(schemaPath, map[string]any{"kind": "demo", "name": "alice", "count": 2}); err != nil {
		t.Fatalf("expected valid input: %v", err)
	}
	if err := ValidateFile(schemaPath, map[string]any{"kind": "other", "name": "Alice", "count": 0}); err == nil {
		t.Fatal("expected enum/pattern/minimum validation error")
	}
}
