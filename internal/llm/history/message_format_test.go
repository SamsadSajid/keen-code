package history_test

import (
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/llm/history"
)

func TestFormatMessageForProviderDoesNotAppendTurnMemory(t *testing.T) {
	message := core.Message{Role: core.RoleAssistant, Content: "Updated the parser.", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}}}}
	if got := history.FormatMessage(message); got != message.Content {
		t.Fatalf("expected assistant content only, got %q", got)
	}
}

func TestFormatMessageForProviderLeavesUserMessageUntouched(t *testing.T) {
	message := core.Message{Role: core.RoleUser, Content: "hello", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{TextOffset: 2, Tool: "read_file", Status: "success"}}}}
	if got := history.FormatMessage(message); got != "hello" {
		t.Fatalf("expected user message to remain unchanged, got %q", got)
	}
}

func TestHistoricalMessageStepsPreservesActivityOrder(t *testing.T) {
	message := core.Message{Role: core.RoleAssistant, Content: "Let me inspect. Found it.", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{TextOffset: 15, Tool: "read_file", Input: map[string]any{"path": "a.go"}, Status: "success"}, {TextOffset: 15, Tool: "grep", Input: map[string]any{"path": "internal", "pattern": "TODO"}, Status: "error"}}}}
	steps := history.MessageSteps(2, message)
	if len(steps) != 2 || steps[0].Text != "Let me inspect." || steps[1].Text != " Found it." || len(steps[0].Activities) != 2 {
		t.Fatalf("unexpected steps: %#v", steps)
	}
	if steps[0].Activities[0].ID != "historical_2_0" || steps[0].Activities[0].Activity.Tool != "read_file" || steps[0].Activities[1].ID != "historical_2_1" || steps[0].Activities[1].Activity.Tool != "grep" {
		t.Fatalf("unexpected activities: %#v", steps[0].Activities)
	}
}

func TestHistoricalToolArgumentsRetainsInput(t *testing.T) {
	activity := core.HistoricalToolActivity{Input: map[string]any{"path": "go.mod", "offset": 2}}
	if got := history.ToolArguments(activity); got != `{"offset":2,"path":"go.mod"}` {
		t.Fatalf("unexpected arguments %q", got)
	}
	if got := history.ToolArguments(core.HistoricalToolActivity{}); got != `{}` {
		t.Fatalf("expected empty arguments, got %q", got)
	}
}

func TestHistoricalToolResultRetainsOnlyCompactOutcome(t *testing.T) {
	exitCode := 1
	for _, tt := range []struct {
		activity core.HistoricalToolActivity
		want     string
	}{
		{core.HistoricalToolActivity{Status: "success"}, `{"status":"success"}`},
		{core.HistoricalToolActivity{Status: "error"}, `{"status":"error"}`},
		{core.HistoricalToolActivity{Status: "success", ExitCode: &exitCode}, `{"status":"success","exit_code":1}`},
	} {
		if got := history.ToolResult(tt.activity); got != tt.want {
			t.Fatalf("unexpected result: want %q, got %q", tt.want, got)
		}
	}
}

func TestHistoricalToolResultRetainsAskUserOutput(t *testing.T) {
	activity := core.HistoricalToolActivity{Tool: "ask_user", Status: "success", RetainedOutput: map[string]any{"answers": []string{"PostgreSQL"}, "cancelled": false}}
	if got := history.ToolResult(activity); got != `{"answers":["PostgreSQL"],"cancelled":false}` {
		t.Fatalf("unexpected retained ask_user result %q", got)
	}
}

func TestFormatMessageForProviderSkipsOffsetInsideUTF8Rune(t *testing.T) {
	message := core.Message{Role: core.RoleAssistant, Content: "é", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{TextOffset: 1, Tool: "read_file", Status: "success"}}}}
	if got := history.FormatMessage(message); got != "é" {
		t.Fatalf("expected invalid UTF-8 boundary to be skipped, got %q", got)
	}
}

func TestHistoricalMessageStepsHandlesBoundaryAndInvalidOffsets(t *testing.T) {
	message := core.Message{Role: core.RoleAssistant, Content: "done", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{TextOffset: -1, Tool: "invalid", Status: "error"}, {TextOffset: 0, Tool: "read_file", Status: "success"}, {TextOffset: 4, Tool: "bash", Input: map[string]any{"command": "go test ./..."}, Status: "success"}, {TextOffset: 5, Tool: "invalid", Status: "error"}}}}
	steps := history.MessageSteps(0, message)
	if len(steps) != 3 || steps[0].Text != "" || steps[0].Activities[0].Activity.Tool != "read_file" || steps[1].Text != "done" || steps[1].Activities[0].Activity.Tool != "bash" || steps[2].Text != "" || len(steps[2].Activities) != 0 {
		t.Fatalf("unexpected steps: %#v", steps)
	}
}
