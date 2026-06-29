package schema

import "testing"

func TestValidateObject(t *testing.T) {
	s := Schema{
		Type:                 "object",
		Required:             []string{"text"},
		AdditionalProperties: false,
		Properties: map[string]Schema{
			"text": {Type: "string"},
		},
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
