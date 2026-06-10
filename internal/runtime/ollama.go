package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OllamaClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewOllama(baseURL string) *OllamaClient {
	return &OllamaClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done             bool `json:"done"`
	EvalCount        int  `json:"eval_count"`
	PromptEvalCount  int  `json:"prompt_eval_count"`
}

func (c *OllamaClient) Chat(ctx context.Context, model string, messages []ChatMessage, stream bool, onDelta StreamCallback) (tokensIn, tokensOut int, err error) {
	body, err := json.Marshal(ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	})
	if err != nil {
		return 0, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, 0, fmt.Errorf("ollama chat: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	if !stream {
		var out ollamaChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return 0, 0, err
		}
		if onDelta != nil && out.Message.Content != "" {
			if err := onDelta(out.Message.Content); err != nil {
				return 0, 0, err
			}
		}
		tokensIn = out.PromptEvalCount
		tokensOut = out.EvalCount
		if tokensIn == 0 {
			tokensIn = estimateInputTokens(messages)
		}
		if tokensOut == 0 {
			tokensOut = EstimateTokens(out.Message.Content)
		}
		return tokensIn, tokensOut, nil
	}

	return parseOllamaStream(resp.Body, messages, onDelta)
}

func parseOllamaStream(r io.Reader, messages []ChatMessage, onDelta StreamCallback) (tokensIn, tokensOut int, err error) {
	scanner := bufio.NewScanner(r)
	var content strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return 0, 0, fmt.Errorf("parse ollama stream line: %w", err)
		}

		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
			if onDelta != nil {
				if err := onDelta(chunk.Message.Content); err != nil {
					return 0, 0, err
				}
			}
		}

		if chunk.Done {
			tokensIn = chunk.PromptEvalCount
			tokensOut = chunk.EvalCount
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}

	if tokensIn == 0 {
		tokensIn = estimateInputTokens(messages)
	}
	if tokensOut == 0 {
		tokensOut = EstimateTokens(content.String())
	}

	return tokensIn, tokensOut, nil
}

func estimateInputTokens(messages []ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += EstimateTokens(m.Content)
	}
	return total
}
