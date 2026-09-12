package llm

import (
	"context"
	"fmt"

	"github.com/mochow13/keen-code/internal/llm/compress"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
	"log/slog"
	"time"
)

func historicalToolActivity(name string, input map[string]any, output, llmOutput any, execErr error) core.HistoricalToolActivity {
	activity := core.HistoricalToolActivity{
		Tool:         name,
		Input:        input,
		HasRawOutput: true,
	}
	if execErr != nil {
		activity.Status = "error"
		activity.RawOutput = map[string]any{"error": execErr.Error()}
		return activity
	}

	activity.Status = "success"
	activity.RawOutput = output
	activity.RetainedOutput = llmOutput
	return activity
}

type toolExecution struct {
	RawOutput any
	LLMOutput any
	Err       error
	Activity  core.HistoricalToolActivity
}

func executeTool(ctx context.Context, registry *tools.Registry, name string, input map[string]any, eventCh chan<- core.StreamEvent) toolExecution {
	start := time.Now()
	rawOutput, llmOutput, err, started := executeValidatedTool(ctx, registry, name, input, eventCh)
	duration := time.Since(start)
	toolCall := &core.ToolCall{Name: name, Input: input, Output: rawOutput, Duration: duration}
	if err != nil {
		toolCall.Error = err.Error()
		slog.Debug("Tool response", "tool", name, "error", err.Error(), "duration", duration)
		if started {
			eventCh <- core.StreamEvent{Type: core.StreamEventTypeToolEnd, ToolCall: toolCall}
		}
	} else {
		slog.Debug("Tool response", "tool", name, "duration", duration)
		eventCh <- core.StreamEvent{Type: core.StreamEventTypeToolEnd, ToolCall: toolCall}
	}
	return toolExecution{
		RawOutput: rawOutput,
		LLMOutput: llmOutput,
		Err:       err,
		Activity:  historicalToolActivity(name, input, rawOutput, llmOutput, err),
	}
}

func executeValidatedTool(
	ctx context.Context,
	registry *tools.Registry,
	name string,
	input map[string]any,
	eventCh chan<- core.StreamEvent,
) (rawOutput, llmOutput any, err error, started bool) {
	if registry == nil {
		return nil, nil, fmt.Errorf("tool registry not available"), false
	}
	tool, exists := registry.Get(name)
	if !exists {
		return nil, nil, fmt.Errorf("tool %q not found", name), false
	}
	if err := tools.ValidateInput(ctx, tool, input); err != nil {
		return nil, nil, err, false
	}
	eventCh <- core.StreamEvent{
		Type: core.StreamEventTypeToolStart,
		ToolCall: &core.ToolCall{
			Name:  name,
			Input: input,
		},
	}
	rawOutput, err = tool.Execute(ctx, input)
	if err != nil {
		return rawOutput, rawOutput, err, true
	}
	return rawOutput, compress.ForLLM(name, rawOutput), nil, true
}
