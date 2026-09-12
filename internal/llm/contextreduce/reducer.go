package contextreduce

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/firebase/genkit/go/ai"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
	openai "github.com/openai/openai-go/v3"
	openaiParam "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const RemovedToolResultPlaceholder = "Tool result removed to fit context."

const ContextWindowExceededError = "context exceeds model window after removing tool results"

var ErrContextWindowExceeded = errors.New("context window exceeded")

type Reduction struct {
	OriginalTokenCount int
	ReducedTokenCount  int
	RemovedToolResults int
	FitsBudget         bool
}

type target struct {
	tokens int
	remove func()
}

func reduce(window, tokens int, targets []target) Reduction {
	budget := core.ContextInputBudget(window)
	r := Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: tokens <= budget}
	placeholder := core.EstimateContextTokenCount(RemovedToolResultPlaceholder)
	for _, target := range targets {
		if r.ReducedTokenCount <= budget {
			break
		}
		if target.tokens <= placeholder {
			continue
		}
		target.remove()
		r.ReducedTokenCount += placeholder - target.tokens
		r.RemovedToolResults++
	}
	r.FitsBudget = r.ReducedTokenCount <= budget
	slog.Debug("Reduced context tool results", "inputTokenCount", r.OriginalTokenCount, "reducedTokenCount", r.ReducedTokenCount, "budgetTokenCount", budget, "removedToolResultCount", r.RemovedToolResults, "fitsBudget", r.FitsBudget)
	return r
}

func marshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func EstimateOpenAI(messages []openai.ChatCompletionMessageParamUnion) int {
	n := 0
	for _, message := range messages {
		n += core.EstimateContextTokenCount(string(marshal(message)))
	}
	return n
}

func ReduceOpenAI(window int, messages []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, Reduction) {
	tokens := EstimateOpenAI(messages)
	if tokens <= core.ContextInputBudget(window) {
		return messages, Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: true}
	}
	retained := map[string]struct{}{}
	for _, message := range messages {
		if message.OfAssistant != nil {
			for _, call := range message.OfAssistant.ToolCalls {
				if call.OfFunction != nil && call.OfFunction.Function.Name == tools.AskUserToolName {
					retained[call.OfFunction.ID] = struct{}{}
				}
			}
		}
	}
	targets := make([]target, 0)
	for i := range messages {
		message := &messages[i]
		if message.OfTool == nil {
			continue
		}
		content := OpenAIToolContent(message.OfTool.Content)
		if content == RemovedToolResultPlaceholder {
			continue
		}
		if _, ok := retained[message.OfTool.ToolCallID]; ok {
			continue
		}
		idx := i
		targets = append(targets, target{tokens: core.EstimateContextTokenCount(content), remove: func() {
			messages[idx].OfTool.Content.OfString = openaiParam.NewOpt(RemovedToolResultPlaceholder)
			messages[idx].OfTool.Content.OfArrayOfContentParts = nil
		}})
	}
	return messages, reduce(window, tokens, targets)
}

func OpenAIToolContent(content openai.ChatCompletionToolMessageParamContentUnion) string {
	if content.OfString.Valid() {
		return content.OfString.Value
	}
	return string(marshal(content))
}

func EstimateResponses(input []responses.ResponseInputItemUnionParam) int {
	n := 0
	for _, item := range input {
		n += core.EstimateContextTokenCount(string(marshal(item)))
	}
	return n
}

func ReduceResponses(window int, input []responses.ResponseInputItemUnionParam) ([]responses.ResponseInputItemUnionParam, Reduction) {
	tokens := EstimateResponses(input)
	if tokens <= core.ContextInputBudget(window) {
		return input, Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: true}
	}
	retained := map[string]struct{}{}
	for _, item := range input {
		if item.OfFunctionCall != nil && item.OfFunctionCall.Name == tools.AskUserToolName {
			retained[item.OfFunctionCall.CallID] = struct{}{}
		}
	}
	targets := make([]target, 0)
	for i := range input {
		item := &input[i]
		if item.OfFunctionCallOutput == nil {
			continue
		}
		content, ok := ResponsesToolOutputContent(item.OfFunctionCallOutput.Output)
		if !ok || content == RemovedToolResultPlaceholder {
			continue
		}
		if _, ok := retained[item.OfFunctionCallOutput.CallID]; ok {
			continue
		}
		idx := i
		targets = append(targets, target{tokens: core.EstimateContextTokenCount(content), remove: func() {
			input[idx].OfFunctionCallOutput.Output.OfString = openaiParam.NewOpt(RemovedToolResultPlaceholder)
			input[idx].OfFunctionCallOutput.Output.OfResponseFunctionCallOutputItemArray = nil
		}})
	}
	return input, reduce(window, tokens, targets)
}

func ResponsesToolOutputContent(output responses.ResponseInputItemFunctionCallOutputOutputUnionParam) (string, bool) {
	if output.OfString.Valid() {
		return output.OfString.Value, true
	}
	if output.OfResponseFunctionCallOutputItemArray != nil {
		return string(marshal(output.OfResponseFunctionCallOutputItemArray)), true
	}
	return "", false
}

func EstimateAnthropic(messages []anthropic.MessageParam) int {
	n := 0
	for _, message := range messages {
		n += core.EstimateContextTokenCount(string(marshal(message)))
	}
	return n
}

func ReduceAnthropic(window int, messages []anthropic.MessageParam) ([]anthropic.MessageParam, Reduction) {
	tokens := EstimateAnthropic(messages)
	if tokens <= core.ContextInputBudget(window) {
		return messages, Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: true}
	}
	retained := map[string]struct{}{}
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && block.OfToolUse.Name == tools.AskUserToolName {
				retained[block.OfToolUse.ID] = struct{}{}
			}
		}
	}
	targets := make([]target, 0)
	for mi := range messages {
		for bi, block := range messages[mi].Content {
			if block.OfToolResult == nil {
				continue
			}
			content := AnthropicToolResultContent(block.OfToolResult)
			if content == RemovedToolResultPlaceholder {
				continue
			}
			if _, ok := retained[block.OfToolResult.ToolUseID]; ok {
				continue
			}
			messageIndex, blockIndex := mi, bi
			targets = append(targets, target{tokens: core.EstimateContextTokenCount(content), remove: func() {
				messages[messageIndex].Content[blockIndex].OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: RemovedToolResultPlaceholder}}}
			}})
		}
	}
	return messages, reduce(window, tokens, targets)
}

func AnthropicToolResultContent(result *anthropic.ToolResultBlockParam) string {
	var b strings.Builder
	for _, content := range result.Content {
		if content.OfText != nil {
			b.WriteString(content.OfText.Text)
		} else {
			b.Write(marshal(content))
		}
	}
	return b.String()
}

func EstimateGenkit(messages []*ai.Message) int {
	n := 0
	for _, message := range messages {
		n += core.EstimateContextTokenCount(string(marshal(message)))
	}
	return n
}

func ReduceGenkit(window int, messages []*ai.Message) ([]*ai.Message, Reduction) {
	tokens := EstimateGenkit(messages)
	if tokens <= core.ContextInputBudget(window) {
		return messages, Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: true}
	}
	targets := make([]target, 0)
	for mi, message := range messages {
		if message != nil && message.Role == ai.RoleTool {
			for pi, part := range message.Content {
				if part == nil || part.ToolResponse == nil || part.ToolResponse.Name == tools.AskUserToolName || part.ToolResponse.Output == RemovedToolResultPlaceholder {
					continue
				}
				messageIndex, partIndex := mi, pi
				targets = append(targets, target{tokens: core.EstimateJSONTokenCount(part.ToolResponse.Output), remove: func() { messages[messageIndex].Content[partIndex].ToolResponse.Output = RemovedToolResultPlaceholder }})
			}
		}
	}
	return messages, reduce(window, tokens, targets)
}

func EstimateBedrock(messages []brtypes.Message) int {
	n := 0
	for _, message := range messages {
		n += core.EstimateContextTokenCount(string(marshal(message)))
	}
	return n
}

func ReduceBedrock(window int, messages []brtypes.Message) ([]brtypes.Message, Reduction) {
	tokens := EstimateBedrock(messages)
	if tokens <= core.ContextInputBudget(window) {
		return messages, Reduction{OriginalTokenCount: tokens, ReducedTokenCount: tokens, FitsBudget: true}
	}
	retained := map[string]struct{}{}
	for _, message := range messages {
		for _, block := range message.Content {
			if toolUse, ok := block.(*brtypes.ContentBlockMemberToolUse); ok && toolUse.Value.Name != nil && *toolUse.Value.Name == tools.AskUserToolName && toolUse.Value.ToolUseId != nil {
				retained[*toolUse.Value.ToolUseId] = struct{}{}
			}
		}
	}
	targets := make([]target, 0)
	for mi := range messages {
		for bi, block := range messages[mi].Content {
			result, ok := block.(*brtypes.ContentBlockMemberToolResult)
			if !ok || result.Value.ToolUseId == nil {
				continue
			}
			content := BedrockToolResultContent(result.Value.Content)
			if content == RemovedToolResultPlaceholder {
				continue
			}
			if _, ok := retained[*result.Value.ToolUseId]; ok {
				continue
			}
			messageIndex, blockIndex := mi, bi
			targets = append(targets, target{tokens: core.EstimateContextTokenCount(content), remove: func() {
				messages[messageIndex].Content[blockIndex].(*brtypes.ContentBlockMemberToolResult).Value.Content = []brtypes.ToolResultContentBlock{&brtypes.ToolResultContentBlockMemberText{Value: RemovedToolResultPlaceholder}}
			}})
		}
	}
	return messages, reduce(window, tokens, targets)
}

func BedrockToolResultContent(content []brtypes.ToolResultContentBlock) string {
	var b strings.Builder
	for _, block := range content {
		if text, ok := block.(*brtypes.ToolResultContentBlockMemberText); ok {
			b.WriteString(text.Value)
		} else {
			b.Write(marshal(block))
		}
	}
	return b.String()
}
