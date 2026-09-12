package llm

import (
	"context"
	"strings"

	"github.com/mochow13/keen-code/internal/llm/core"
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

type compactionTestClient struct {
	request []core.Message
	opts    core.StreamOptions
	events  []core.StreamEvent
}

func (c *compactionTestClient) StreamChat(_ context.Context, messages []core.Message, _ *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	c.request = core.CloneMessages(messages)
	c.opts = streamOptions(opts)
	ch := make(chan core.StreamEvent, len(c.events))
	for _, event := range c.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}
func (*compactionTestClient) Reset() {}

func TestAutoCompactBuildsPrivateReplacement(t *testing.T) {
	client := &compactionTestClient{events: []core.StreamEvent{{Type: core.StreamEventTypeChunk, Content: "## Goal\nContinue work"}}}
	history := []core.Message{{Role: core.RoleSystem, Content: "agent prompt"}, {Role: core.RoleUser, Content: "implement this exactly"}}
	replacement, _, err := AutoCompact(context.Background(), client, history, "session")
	if err != nil {
		t.Fatal(err)
	}
	if !client.opts.OneShot || !client.opts.DisableAutoCompaction || client.opts.SessionID != "session" {
		t.Fatalf("unexpected nested options: %#v", client.opts)
	}
	if len(client.request) < 2 || client.request[0].Role != core.RoleSystem || strings.Contains(client.request[1].Content, "agent prompt") {
		t.Fatalf("system history was not excluded: %#v", client.request)
	}
	if len(replacement) != 2 || replacement[0].Role != core.RoleSystem || replacement[0].Content != "agent prompt" || replacement[1].Role != core.RoleUser || !strings.Contains(replacement[1].Content, "implement this exactly") {
		t.Fatalf("invalid replacement: %#v", replacement)
	}
	replacement[0].Content = "changed"
	if history[0].Content != "agent prompt" {
		t.Fatal("replacement system message aliases original history")
	}
}

func TestAutoCompactIsTransactional(t *testing.T) {
	history := []core.Message{{Role: core.RoleUser, Content: "task"}}
	client := &compactionTestClient{}
	_, _, err := AutoCompact(context.Background(), client, history, "")
	if err == nil || !strings.Contains(err.Error(), "empty summary") {
		t.Fatalf("expected empty summary error, got %v", err)
	}
	if history[0].Content != "task" {
		t.Fatal("history mutated")
	}
}

func TestContextBudgetAndAutoCompactionThreshold(t *testing.T) {
	if got := core.ContextInputBudget(100000); got != 95000 {
		t.Fatalf("budget = %d, want 95000", got)
	}
	if !core.ShouldAutoCompact(85500, 95000) || core.ShouldAutoCompact(85499, 95000) {
		t.Fatal("unexpected 90% trigger boundary")
	}
}
