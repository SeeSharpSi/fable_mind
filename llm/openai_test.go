package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleClientGenerate(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}]}`))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(server.URL+"/v1", "test-key", "test-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}
	defer client.Close()

	response, err := client.Generate(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response != `{"ok":true}` {
		t.Errorf("Generate() = %q, want JSON response", response)
	}
	if received.Model != "test-model" {
		t.Errorf("model = %q, want %q", received.Model, "test-model")
	}
	if len(received.Messages) != 2 || received.Messages[0].Role != "system" || received.Messages[1].Role != "user" {
		t.Errorf("messages = %#v, want system and user messages", received.Messages)
	}
	if received.ResponseFormat.Type != "json_object" {
		t.Errorf("response format = %q, want json_object", received.ResponseFormat.Type)
	}
	if received.Temperature != 0.9 {
		t.Errorf("temperature = %v, want 0.9", received.Temperature)
	}
}

func TestChatCompletionsEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "base API URL",
			baseURL: "https://example.com/v1/",
			want:    "https://example.com/v1/chat/completions",
		},
		{
			name:    "full endpoint",
			baseURL: "https://example.com/v1/chat/completions",
			want:    "https://example.com/v1/chat/completions",
		},
		{
			name:    "query parameter",
			baseURL: "https://example.com/openai/deployments/model?api-version=2024-10-21",
			want:    "https://example.com/openai/deployments/model/chat/completions?api-version=2024-10-21",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := chatCompletionsEndpoint(test.baseURL)
			if err != nil {
				t.Fatalf("chatCompletionsEndpoint() error = %v", err)
			}
			if got != test.want {
				t.Errorf("chatCompletionsEndpoint() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOpenAICompatibleClientReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(server.URL, "", "test-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}
	defer client.Close()

	_, err = client.Generate(context.Background(), "", "prompt")
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("Generate() error = %v, want API error message", err)
	}
}

func TestNewFromEnvOpenAI(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_BASE_URL", "https://example.com/v1")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")

	client, err := NewFromEnv(context.Background())
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	defer client.Close()

	if client.Provider() != "openai-compatible" {
		t.Errorf("Provider() = %q, want openai-compatible", client.Provider())
	}
}

func TestNewFromEnvRejectsUnknownProvider(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "unknown")

	_, err := NewFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported LLM_PROVIDER") {
		t.Fatalf("NewFromEnv() error = %v, want unsupported provider error", err)
	}
}
