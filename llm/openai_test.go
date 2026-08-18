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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":21,"completion_tokens":7,"total_tokens":28}}`))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(server.URL+"/v1", "test-key", "test-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}
	defer client.Close()

	response, err := client.Generate(context.Background(), "system prompt", "user prompt", GenerationOptions{MaxOutputTokens: 1200})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Text != `{"ok":true}` {
		t.Errorf("Generate() = %q, want JSON response", response.Text)
	}
	if response.Stats.Usage != (TokenUsage{PromptTokens: 21, CompletionTokens: 7, TotalTokens: 28}) {
		t.Errorf("usage = %#v, want endpoint token counts", response.Stats.Usage)
	}
	if response.Stats.Requests != 1 || response.Stats.ResponsesWithUsage != 1 {
		t.Errorf("request stats = %#v, want one request with usage", response.Stats)
	}
	if response.Stats.InputCharacters != len("system prompt")+len("user prompt") {
		t.Errorf("input characters = %d, want prompt character count", response.Stats.InputCharacters)
	}
	if response.Stats.OutputCharacters != len(response.Text) {
		t.Errorf("output characters = %d, want generated text length", response.Stats.OutputCharacters)
	}
	if response.Stats.Duration <= 0 {
		t.Errorf("duration = %v, want positive duration", response.Stats.Duration)
	}
	if received.Model != "test-model" {
		t.Errorf("model = %q, want %q", received.Model, "test-model")
	}
	if len(received.Messages) != 2 || received.Messages[0].Role != "system" || received.Messages[1].Role != "user" {
		t.Errorf("messages = %#v, want system and user messages", received.Messages)
	}
	if received.ResponseFormat == nil || received.ResponseFormat.Type != "json_object" {
		t.Errorf("response format = %#v, want json_object", received.ResponseFormat)
	}
	if received.Temperature != defaultTemperature {
		t.Errorf("temperature = %v, want %v", received.Temperature, defaultTemperature)
	}
	if received.MaxTokens != 1200 {
		t.Errorf("max_tokens = %d, want 1200", received.MaxTokens)
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

	_, err = client.Generate(context.Background(), "", "prompt", GenerationOptions{})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("Generate() error = %v, want API error message", err)
	}
}

func TestNewFromEnv(t *testing.T) {
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

func TestNewFromEnvRequiresModel(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "")

	_, err := NewFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "OPENAI_MODEL is required") {
		t.Fatalf("NewFromEnv() error = %v, want missing model error", err)
	}
}

func TestOpenAICompatibleClientOmitsEmptyAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no header", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer server.Close()

	client, err := NewOpenAICompatibleClient(server.URL, "", "local-model")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleClient() error = %v", err)
	}
	defer client.Close()

	if _, err := client.Generate(context.Background(), "", "prompt", GenerationOptions{}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestNewFromEnvTemperature(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_TEMPERATURE", "0.4")

	client, err := NewFromEnv(context.Background())
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	defer client.Close()

	openAIClient := client.(*openAICompatibleClient)
	if openAIClient.temperature != 0.4 {
		t.Errorf("temperature = %v, want 0.4", openAIClient.temperature)
	}
}

func TestNewFromEnvRejectsInvalidTemperature(t *testing.T) {
	for _, temperature := range []string{"high", "NaN", "+Inf", "-0.1", "2.1"} {
		t.Run(temperature, func(t *testing.T) {
			t.Setenv("OPENAI_MODEL", "test-model")
			t.Setenv("OPENAI_TEMPERATURE", temperature)

			_, err := NewFromEnv(context.Background())
			if err == nil || !strings.Contains(err.Error(), "OPENAI_TEMPERATURE") {
				t.Fatalf("NewFromEnv() error = %v, want temperature validation error", err)
			}
		})
	}
}

func TestNewFromEnvCanDisableResponseFormat(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_RESPONSE_FORMAT", "none")

	client, err := NewFromEnv(context.Background())
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	defer client.Close()

	if got := client.(*openAICompatibleClient).responseFormat; got != "none" {
		t.Errorf("response format = %q, want none", got)
	}
}

func TestNewFromEnvRejectsInvalidResponseFormat(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_RESPONSE_FORMAT", "xml")

	_, err := NewFromEnv(context.Background())
	if err == nil || !strings.Contains(err.Error(), "OPENAI_RESPONSE_FORMAT") {
		t.Fatalf("NewFromEnv() error = %v, want response format validation error", err)
	}
}

func TestOpenAICompatibleClientOmitsDisabledResponseFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.ResponseFormat != nil {
			t.Errorf("response_format = %#v, want omitted", request.ResponseFormat)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer server.Close()

	client, err := newOpenAICompatibleClient(server.URL, "", "test-model", defaultTemperature, "none")
	if err != nil {
		t.Fatalf("newOpenAICompatibleClient() error = %v", err)
	}
	defer client.Close()

	if _, err := client.Generate(context.Background(), "system", "prompt", GenerationOptions{}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
}
