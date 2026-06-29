package analyzer

import "testing"

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
