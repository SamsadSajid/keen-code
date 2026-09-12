package llm

import (
	"context"
	"strings"

	"github.com/mochow13/keen-code/internal/llm/compaction"
	"github.com/mochow13/keen-code/internal/llm/core"
)

func AutoCompact(ctx context.Context, client LLMClient, history []core.Message, sessionID string) ([]core.Message, *core.TokenUsage, error) {
	prompt := BuildAutoCompactionPrompt()
	request, err := compaction.BuildRequest(history, prompt, true)
	if err != nil {
		return nil, nil, err
	}
	events, err := client.StreamChat(ctx, request, nil, core.StreamOptions{
		SessionID:             sessionID,
		OneShot:               true,
		DisableAutoCompaction: true,
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
		case core.StreamEventTypeUsage:
			usage = event.Usage
		case core.StreamEventTypeError:
			if event.Error != nil {
				return nil, usage, event.Error
			}
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
