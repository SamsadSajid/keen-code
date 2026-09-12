package llm

import (
	"context"
	"strings"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

type LLMClient interface {
	StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error)
	Reset()
}

func streamOptions(opts []core.StreamOptions) core.StreamOptions {
	if len(opts) == 0 {
		return core.StreamOptions{}
	}
	return opts[0]
}

func opencodeSessionID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return strings.ReplaceAll(sessionID, "-", "")
}

func promptCacheKey(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return strings.ReplaceAll(sessionID, "-", "")
}
