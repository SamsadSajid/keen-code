package approval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

type testClient struct {
	events      []core.StreamEvent
	err         error
	open        bool
	calls       int
	ctx         context.Context
	messages    []core.Message
	onCall      func()
	registrySet bool
	opts        []core.StreamOptions
}

func (c *testClient) StreamChat(ctx context.Context, messages []core.Message, registry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	c.calls++
	c.ctx = ctx
	c.messages = messages
	c.registrySet = registry != nil
	c.opts = opts
	if c.onCall != nil {
		c.onCall()
	}
	if c.err != nil {
		return nil, c.err
	}
	ch := make(chan core.StreamEvent, len(c.events))
	for _, event := range c.events {
		ch <- event
	}
	if !c.open {
		close(ch)
	}
	return ch, nil
}
func (*testClient) Reset() {}

func TestReviewerApprovesOnlyExactCompletedResponse(t *testing.T) {
	client := &testClient{events: []core.StreamEvent{{Type: core.StreamEventTypeChunk, Content: `{"decision":"approve"}`}, {Type: core.StreamEventTypeDone}}}
	got, err := New(client).ReviewOperation(context.Background(), tools.Operation{Kind: "bash", Command: "go test ./..."})
	if err != nil || got != tools.OperationReviewApproved {
		t.Fatalf("ReviewOperation() = %v, %v", got, err)
	}
	if client.registrySet || len(client.opts) != 1 || !client.opts[0].OneShot || !client.opts[0].DisableToolCalls || !client.opts[0].DisableAutoCompaction {
		t.Fatal("review request was not isolated")
	}
	if len(client.messages) != 2 || client.messages[0].Role != core.RoleSystem || client.messages[0].Content != systemPrompt || client.messages[1].Role != core.RoleUser || client.opts[0].SessionID != "" {
		t.Fatal("review contains extra context")
	}
	var decoded tools.Operation
	if err := json.Unmarshal([]byte(client.messages[1].Content), &decoded); err != nil || decoded.Kind != "bash" || decoded.Command != "go test ./..." {
		t.Fatalf("operation changed: %+v, %v", decoded, err)
	}
	deadline, ok := client.ctx.Deadline()
	if !ok || time.Until(deadline) > reviewTimeout || !errors.Is(client.ctx.Err(), context.Canceled) {
		t.Fatal("review context was not bounded and closed")
	}
}

func TestReviewerRejectsPartialOrExtraResponse(t *testing.T) {
	for _, events := range [][]core.StreamEvent{
		{{Type: core.StreamEventTypeChunk, Content: `{"decision":"approve"}`}},
		{{Type: core.StreamEventTypeChunk, Content: `{"decision":"approve"} extra`}, {Type: core.StreamEventTypeDone}},
		{{Type: core.StreamEventTypeToolStart}, {Type: core.StreamEventTypeDone}},
	} {
		got, _ := New(&testClient{events: events}).ReviewOperation(context.Background(), tools.Operation{Kind: "bash", Command: "pwd"})
		if got != tools.OperationReviewAskUser {
			t.Fatalf("ReviewOperation() = %v, want ask user", got)
		}
	}
}

func completedResponse(text string) []core.StreamEvent {
	return []core.StreamEvent{{Type: core.StreamEventTypeChunk, Content: text}, {Type: core.StreamEventTypeDone}}
}

func TestReviewerResponseSchema(t *testing.T) {
	tests := []struct {
		name, text string
		approve    bool
	}{
		{"approve", `{"decision":"approve"}`, true},
		{"whitespace", " \n { \"decision\" : \"approve\" }\t", true},
		{"ask", `{"decision":"ask_user"}`, false},
		{"empty", "", false},
		{"unknown", `{"decision":"yes"}`, false},
		{"extra field", `{"decision":"approve","reason":"safe"}`, false},
		{"duplicate key", `{"decision":"ask_user","decision":"approve"}`, false},
		{"duplicate same value", `{"decision":"approve","decision":"approve"}`, false},
		{"wrong case", `{"Decision":"approve"}`, false},
		{"wrong type", `{"decision":true}`, false},
		{"array", `[{"decision":"approve"}]`, false},
		{"null", `null`, false},
		{"trailing text", `{"decision":"approve"} safe`, false},
		{"multiple objects", `{"decision":"approve"}{"decision":"approve"}`, false},
		{"markdown", "```json\n{\"decision\":\"approve\"}\n```", false},
		{"oversize", strings.Repeat(" ", maxResponseSize) + `{"decision":"approve"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &testClient{events: completedResponse(tt.text)}
			got, _ := New(client).ReviewOperation(context.Background(), tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
			if (got == tools.OperationReviewApproved) != tt.approve {
				t.Fatalf("decision = %v", got)
			}
		})
	}
}

func TestReviewerStreamFailures(t *testing.T) {
	for _, terminal := range []core.StreamEventType{core.StreamEventTypeIncomplete, core.StreamEventTypeError, core.StreamEventTypeToolStart, core.StreamEventTypeToolEnd, core.StreamEventTypeAutoCompactionStarted, "unknown"} {
		t.Run(string(terminal), func(t *testing.T) {
			client := &testClient{events: []core.StreamEvent{{Type: core.StreamEventTypeChunk, Content: `{"decision":"approve"}`}, {Type: terminal}, {Type: core.StreamEventTypeDone}}}
			got, _ := New(client).ReviewOperation(context.Background(), tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
			if got != tools.OperationReviewAskUser {
				t.Fatal("invalid stream granted approval")
			}
		})
	}
	providerErr := errors.New("provider failed")
	for _, client := range []*testClient{{err: providerErr}, {events: []core.StreamEvent{{Type: core.StreamEventTypeError, Error: providerErr}}}, {events: []core.StreamEvent{{Type: core.StreamEventTypeChunk, Content: `{"decision":"approve"}`}}}} {
		got, err := New(client).ReviewOperation(context.Background(), tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
		if got != tools.OperationReviewAskUser || err == nil {
			t.Fatalf("failed stream = %v, %v", got, err)
		}
	}
}

func TestReviewerCancelsWithoutStreamClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	got, err := New(&testClient{open: true}).ReviewOperation(ctx, tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
	if got != tools.OperationReviewAskUser || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked stream = %v, %v", got, err)
	}
}

func TestReviewerNeverApprovesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &testClient{events: completedResponse(`{"decision":"approve"}`), onCall: cancel}
	got, err := New(client).ReviewOperation(ctx, tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
	if got != tools.OperationReviewAskUser || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled review = %v, %v", got, err)
	}
	client.calls = 0
	_, _ = New(client).ReviewOperation(ctx, tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
	if client.calls != 0 {
		t.Fatal("canceled request reached the model")
	}
}

func TestReviewerSkipsIneligibleInput(t *testing.T) {
	for _, operation := range []tools.Operation{
		{Kind: "call_mcp_tool", Content: "private arguments"}, {Kind: "unknown"},
		{Kind: tools.BashToolName, Command: " "}, {Kind: tools.BashToolName, Command: "rm -rf build"},
		{Kind: tools.BashToolName, Command: "echo token=examplecredential"},
		{Kind: tools.BashToolName, Command: strings.Repeat("x", maxCommandSize+1)},
		{Kind: tools.BashToolName, Command: "echo \xff"},
		{Kind: tools.WriteFileToolName, Content: "x"},
		{Kind: tools.WriteFileToolName, Path: "/workspace/a", Content: "token=examplecredential"},
		{Kind: tools.EditFileToolName, Path: "/workspace/a", Content: strings.Repeat("x", maxOperationSize)},
		{Kind: tools.WriteFileToolName, Path: "/workspace/a", Content: strings.Repeat("\x00", maxOperationSize/2)},
	} {
		client := &testClient{events: completedResponse(`{"decision":"approve"}`)}
		got, _ := New(client).ReviewOperation(context.Background(), operation)
		if got != tools.OperationReviewAskUser || client.calls != 0 {
			t.Fatalf("ineligible %s input reached model or granted approval", operation.Kind)
		}
	}
}

func TestReviewerApprovesBoundedFileChange(t *testing.T) {
	for _, kind := range []string{tools.WriteFileToolName, tools.EditFileToolName} {
		client := &testClient{events: completedResponse(`{"decision":"approve"}`)}
		got, err := New(client).ReviewOperation(context.Background(), tools.Operation{Kind: kind, Path: "/workspace/main.go", Content: "package main\n", Bytes: 13})
		if err != nil || got != tools.OperationReviewApproved || client.calls != 1 {
			t.Fatalf("file review = %v, %v", got, err)
		}
	}
}

func TestReviewerUnavailable(t *testing.T) {
	for _, reviewer := range []*Reviewer{nil, New(nil)} {
		got, err := reviewer.ReviewOperation(context.Background(), tools.Operation{Kind: tools.BashToolName, Command: "pwd"})
		if err != nil || got != tools.OperationReviewAskUser {
			t.Fatalf("unavailable = %v, %v", got, err)
		}
	}
}
