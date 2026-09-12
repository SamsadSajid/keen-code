package session

import "github.com/mochow13/keen-code/internal/llm/core"

func BuildConversation(events []Event) []core.Message {
	var messages []core.Message

	for _, event := range events {
		switch event.Kind {
		case KindUserMessage:
			if event.UserMessage != nil {
				messages = append(messages, core.Message{
					Role:    core.RoleUser,
					Content: event.UserMessage.Content,
				})
			}
		case KindAssistantTurn:
			if event.AssistantTurn != nil && (event.AssistantTurn.Message != "" || !event.AssistantTurn.TurnMemory.IsEmpty()) {
				messages = append(messages, core.Message{
					Role:       core.RoleAssistant,
					Content:    event.AssistantTurn.Message,
					TurnMemory: core.CloneTurnMemory(event.AssistantTurn.TurnMemory),
				})
			}
		case KindCompactionApplied:
			if event.CompactionApplied != nil {
				messages = cloneMessages(event.CompactionApplied.Messages)
			}
		}
	}

	return messages
}
