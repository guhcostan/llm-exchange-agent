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

type VLLMClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewVLLM(baseURL string) *VLLMClient {
	return &VLLMClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

type vllmChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type vllmChatResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *VLLMClient) Chat(ctx context.Context, model string, messages []ChatMessage, stream bool, onDelta StreamCallback) (tokensIn, tokensOut int, err error) {
	body, err := json.Marshal(vllmChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   stream,
	})
	if err != nil {
		return 0, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
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
		return 0, 0, fmt.Errorf("vllm chat: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	if !stream {
		var out vllmChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return 0, 0, err
		}
		content := ""
		if len(out.Choices) > 0 {
			content = out.Choices[0].Message.Content
		}
		if onDelta != nil && content != "" {
			if err := onDelta(content); err != nil {
				return 0, 0, err
			}
		}
		tokensIn = out.Usage.PromptTokens
		tokensOut = out.Usage.CompletionTokens
		if tokensIn == 0 {
			tokensIn = estimateInputTokens(messages)
		}
		if tokensOut == 0 {
			tokensOut = EstimateTokens(content)
		}
		return tokensIn, tokensOut, nil
	}

	return parseVLLMStream(resp.Body, messages, onDelta)
}

func parseVLLMStream(r io.Reader, messages []ChatMessage, onDelta StreamCallback) (tokensIn, tokensOut int, err error) {
	scanner := bufio.NewScanner(r)
	var content strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk vllmChatResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return 0, 0, fmt.Errorf("parse vllm stream chunk: %w", err)
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				content.WriteString(delta)
				if onDelta != nil {
					if err := onDelta(delta); err != nil {
						return 0, 0, err
					}
				}
			}
		}

		if chunk.Usage.PromptTokens > 0 {
			tokensIn = chunk.Usage.PromptTokens
		}
		if chunk.Usage.CompletionTokens > 0 {
			tokensOut = chunk.Usage.CompletionTokens
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
