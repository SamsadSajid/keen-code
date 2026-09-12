package core

import "testing"

func TestCloneTurnMemoryClonesCompressedRetainedOutput(t *testing.T) {
	original := &TurnMemory{ToolActivity: []HistoricalToolActivity{{
		Tool: "grep",
		RetainedOutput: map[string]any{
			"common_prefix": "internal/llm/",
			"matches": map[string][]map[string]any{
				"message.go": {{"line": "original"}},
			},
		},
	}}}

	cloned := CloneTurnMemory(original)
	retained := cloned.ToolActivity[0].RetainedOutput.(map[string]any)
	matches := retained["matches"].(map[string][]map[string]any)
	matches["message.go"][0]["line"] = "changed"
	matches["other.go"] = []map[string]any{{"line": "new"}}

	originalMatches := original.ToolActivity[0].RetainedOutput.(map[string]any)["matches"].(map[string][]map[string]any)
	if got := originalMatches["message.go"][0]["line"]; got != "original" {
		t.Fatalf("original nested match mutated: %v", got)
	}
	if _, exists := originalMatches["other.go"]; exists {
		t.Fatal("original matches map mutated")
	}
}

func TestCloneTurnMemoryClonesHistoricalActivity(t *testing.T) {
	original := &TurnMemory{ToolActivity: []HistoricalToolActivity{{
		Tool:   "call_mcp_tool",
		Input:  map[string]any{"arguments": map[string]any{"query": "original"}, "values": []any{"first"}},
		Status: "success",
	}}}
	cloned := CloneTurnMemory(original)
	if cloned == nil || cloned.IsEmpty() {
		t.Fatalf("expected non-empty clone, got %#v", cloned)
	}
	cloned.ToolActivity[0].Tool = "grep"
	cloned.ToolActivity[0].Input["arguments"].(map[string]any)["query"] = "changed"
	cloned.ToolActivity[0].Input["values"].([]any)[0] = "changed"
	if original.ToolActivity[0].Tool != "call_mcp_tool" || original.ToolActivity[0].Input["arguments"].(map[string]any)["query"] != "original" || original.ToolActivity[0].Input["values"].([]any)[0] != "first" {
		t.Fatalf("expected independent clone, got %#v", original.ToolActivity)
	}
}
