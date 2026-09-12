package history

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSerializeToolOutputPreservesHTMLCharacters(t *testing.T) {
	content := `if a < b && b > c { println("ok") }`
	got := SerializeJSON(map[string]any{"content": content})

	if !json.Valid([]byte(got)) {
		t.Fatalf("SerializeJSON() returned invalid JSON: %q", got)
	}
	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`} {
		if strings.Contains(got, escaped) {
			t.Fatalf("SerializeJSON() contains HTML escape %q: %s", escaped, got)
		}
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if decoded["content"] != content {
		t.Fatalf("content = %q, want %q", decoded["content"], content)
	}
}

func TestSerializeToolOutputNilAndUnsupportedValues(t *testing.T) {
	if got := SerializeJSON(nil); got != "{}" {
		t.Fatalf("SerializeJSON(nil) = %q, want %q", got, "{}")
	}
	if got := SerializeJSON(make(chan int)); got != "{}" {
		t.Fatalf("SerializeJSON(channel) = %q, want %q", got, "{}")
	}
}
