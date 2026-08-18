// Package llm provides the language model backends used by the application.
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	defaultGeminiModel = "gemini-3.1-flash-lite-preview"
	defaultOpenAIURL   = "https://api.openai.com/v1"
)

// Client generates text using a configured language model provider.
type Client interface {
	Generate(ctx context.Context, systemInstruction, prompt string) (string, error)
	Provider() string
	Close() error
}

// NewFromEnv creates the provider selected by LLM_PROVIDER.
func NewFromEnv(ctx context.Context) (Client, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if provider == "" {
		provider = "gemini"
	}

	switch provider {
	case "gemini":
		apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if apiKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required when LLM_PROVIDER=gemini")
		}

		model := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
		if model == "" {
			model = defaultGeminiModel
		}
		return NewGeminiClient(ctx, apiKey, model)

	case "openai", "openai-compatible":
		model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
		if model == "" {
			return nil, fmt.Errorf("OPENAI_MODEL is required when LLM_PROVIDER=%s", provider)
		}

		baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
		if baseURL == "" {
			baseURL = defaultOpenAIURL
		}
		return NewOpenAICompatibleClient(baseURL, os.Getenv("OPENAI_API_KEY"), model)

	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER %q: use gemini or openai", provider)
	}
}
