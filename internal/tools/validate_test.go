package tools

import "testing"

func TestValidateArgs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string"},
			"offset":    map[string]any{"type": "integer"},
			"flag":      map[string]any{"type": "boolean"},
		},
		"required":             []any{"file_path"},
		"additionalProperties": false,
	}
	if err := ValidateArgs(schema, map[string]any{"file_path": "a.go"}); err != nil {
		t.Errorf("valid args rejected: %v", err)
	}
	if err := ValidateArgs(schema, map[string]any{}); err == nil {
		t.Error("missing required file_path should fail")
	}
	if err := ValidateArgs(schema, map[string]any{"file_path": 3}); err == nil {
		t.Error("wrong type should fail")
	}
	if err := ValidateArgs(schema, map[string]any{"file_path": "a.go", "offset": 1.5}); err == nil {
		t.Error("non-integer offset should fail")
	}
	if err := ValidateArgs(schema, map[string]any{"file_path": "a.go", "unknown": 1}); err == nil {
		t.Error("unknown property should fail with additionalProperties=false")
	}
}
