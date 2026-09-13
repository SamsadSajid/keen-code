package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/mochow13/keen-code/internal/llm/compaction"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

func AutoCompact(ctx context.Context, client LLMClient, history []core.Message, toolRegistry *tools.Registry, sessionID string) ([]core.Message, *core.TokenUsage, error) {
	request, err := compaction.BuildRequest(history, BuildAutoCompactionPrompt(), true)
	if err != nil {
		return nil, nil, err
	}
	events, err := client.StreamChat(ctx, request, toolRegistry, core.StreamOptions{
		SessionID:             sessionID,
		OneShot:               true,
		DisableAutoCompaction: true,
		DisableToolCalls:      true,
	})
	if err != nil {
		return nil, nil, err
	}

	var summary strings.Builder
	var usage *core.TokenUsage

	for event := range events {
		switch event.Type {
		case core.StreamEventTypeChunk:
			summary.WriteString(event.Content)
		case core.StreamEventTypeToolStart, core.StreamEventTypeToolEnd:
			summary.Reset()
		case core.StreamEventTypeUsage:
			usage = event.Usage
		case core.StreamEventTypeError, core.StreamEventTypeIncomplete:
			if event.Error != nil {
				return nil, usage, event.Error
			}
			return nil, usage, fmt.Errorf("automatic compaction stream incomplete")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, usage, err
	}
	replacement, err := compaction.AutomaticReplacement(summary.String(), history)
	if err != nil {
		return nil, usage, err
	}
	return replacement, usage, nil
}
