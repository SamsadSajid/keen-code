package history

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mochow13/keen-code/internal/llm/core"
)

type MessageStep struct {
	Text       string
	Activities []ToolInvocation
}

type ToolInvocation struct {
	ID       string
	Activity core.HistoricalToolActivity
}

func FormatMessage(message core.Message) string {
	return message.Content
}

func MessageSteps(messageIndex int, message core.Message) []MessageStep {
	if message.Role != core.RoleAssistant || message.TurnMemory == nil || len(message.TurnMemory.ToolActivity) == 0 {
		return []MessageStep{{Text: FormatMessage(message)}}
	}

	steps := make([]MessageStep, 0, len(message.TurnMemory.ToolActivity)+1)
	cursor := 0
	activityIndex := 0
	for _, activity := range message.TurnMemory.ToolActivity {
		if activity.TextOffset < cursor || activity.TextOffset > len(message.Content) || activity.Tool == "" {
			continue
		}
		if activity.TextOffset > 0 && activity.TextOffset < len(message.Content) && !utf8.RuneStart(message.Content[activity.TextOffset]) {
			continue
		}

		invocation := ToolInvocation{
			ID:       "historical_" + strconv.Itoa(messageIndex) + "_" + strconv.Itoa(activityIndex),
			Activity: activity,
		}
		activityIndex++

		if len(steps) > 0 && activity.TextOffset == cursor && len(steps[len(steps)-1].Activities) > 0 {
			steps[len(steps)-1].Activities = append(steps[len(steps)-1].Activities, invocation)
			continue
		}

		steps = append(steps, MessageStep{
			Text:       message.Content[cursor:activity.TextOffset],
			Activities: []ToolInvocation{invocation},
		})
		cursor = activity.TextOffset
	}

	if len(steps) == 0 {
		return []MessageStep{{Text: FormatMessage(message)}}
	}

	finalMessage := message
	finalMessage.Content = message.Content[cursor:]
	steps = append(steps, MessageStep{Text: FormatMessage(finalMessage)})
	return steps
}

func ToolInput(activity core.HistoricalToolActivity) map[string]any {
	if activity.Input == nil {
		return map[string]any{}
	}
	return activity.Input
}

func ToolArguments(activity core.HistoricalToolActivity) string {
	return SerializeJSON(ToolInput(activity))
}

func ToolResult(activity core.HistoricalToolActivity) string {
	if activity.RetainedOutput != nil {
		return SerializeJSON(activity.RetainedOutput)
	}
	if activity.HasRawOutput {
		return SerializeJSON(activity.RawOutput)
	}

	status := activity.Status
	if status != "success" {
		status = "error"
	}
	result := struct {
		Status   string `json:"status"`
		ExitCode *int   `json:"exit_code,omitempty"`
	}{
		Status:   status,
		ExitCode: activity.ExitCode,
	}
	return SerializeJSON(result)
}

func SerializeJSON(v any) string {
	if v == nil {
		v = map[string]any{}
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return "{}"
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
