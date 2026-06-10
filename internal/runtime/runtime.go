package runtime

import "context"

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamCallback func(delta string) error

type Client interface {
	Chat(ctx context.Context, model string, messages []ChatMessage, stream bool, onDelta StreamCallback) (tokensIn, tokensOut int, err error)
}

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	n := len(text) / 4
	if n < 1 {
		return 1
	}
	return n
}
