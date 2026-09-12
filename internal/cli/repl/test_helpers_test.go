package repl

import (
	"context"
	"github.com/mochow13/keen-code/internal/llm/core"

	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/tools"
)

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry) (<-chan core.StreamEvent, error)
	resetCount     int
}

func (m *mockLLMClient) StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	if m.streamChatFunc != nil {
		return m.streamChatFunc(ctx, messages, toolRegistry)
	}
	ch := make(chan core.StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockLLMClient) Reset() {
	m.resetCount++
}

func processCmd(m replModel, cmd tea.Cmd) (replModel, tea.Cmd) {
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m, _ = processCmd(m, c)
			}
		}
		return m, nil
	}
	return m.updateNormalMode(msg)
}
