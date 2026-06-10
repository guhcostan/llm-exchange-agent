package runtime_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-share/agent/internal/runtime"
)

func TestVLLMNonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"hi"}}],
			"usage":{"prompt_tokens":3,"completion_tokens":1}
		}`))
	}))
	defer srv.Close()

	client := runtime.NewVLLM(srv.URL)
	tin, tout, err := client.Chat(context.Background(), "m", []runtime.ChatMessage{
		{Role: "user", Content: "hello"},
	}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tin != 3 || tout != 1 {
		t.Fatalf("tokens %d %d", tin, tout)
	}
}
