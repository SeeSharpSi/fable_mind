// Package llm provides the language model backends used by the application.
package llm

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOpenAIURL      = "https://api.openai.com/v1"
	defaultTemperature    = 0.65
	maxModelTemperature   = 2.0
	defaultResponseFormat = "json_object"
)

// TokenUsage contains token counts reported by an OpenAI-compatible endpoint.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// GenerationStats describes work performed for one or more model requests.
type GenerationStats struct {
	Usage              TokenUsage
	Duration           time.Duration
	InputCharacters    int
	OutputCharacters   int
	Requests           int
	ResponsesWithUsage int
}

// Add combines generation statistics, including retry requests.
func (s GenerationStats) Add(other GenerationStats) GenerationStats {
	s.Usage.PromptTokens += other.Usage.PromptTokens
	s.Usage.CompletionTokens += other.Usage.CompletionTokens
	s.Usage.TotalTokens += other.Usage.TotalTokens
	s.Duration += other.Duration
	s.InputCharacters += other.InputCharacters
	s.OutputCharacters += other.OutputCharacters
	s.Requests += other.Requests
	s.ResponsesWithUsage += other.ResponsesWithUsage
	return s
}

// Generation contains model output and its request statistics.
type Generation struct {
	Text  string
	Stats GenerationStats
}

// GenerationOptions controls one model request.
type GenerationOptions struct {
	MaxOutputTokens int
}

// Client generates text using a configured language model provider. Generate
// must return available request statistics even when generation fails.
type Client interface {
	Generate(ctx context.Context, systemInstruction, prompt string, options GenerationOptions) (Generation, error)
	Provider() string
	Close() error
}

// NewFromEnv creates an OpenAI-compatible client from environment variables.
func NewFromEnv(_ context.Context) (Client, error) {
	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		return nil, fmt.Errorf("OPENAI_MODEL is required")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultOpenAIURL
	}

	temperature := defaultTemperature
	if rawTemperature := strings.TrimSpace(os.Getenv("OPENAI_TEMPERATURE")); rawTemperature != "" {
		parsed, err := strconv.ParseFloat(rawTemperature, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > maxModelTemperature {
			return nil, fmt.Errorf("OPENAI_TEMPERATURE must be a number from 0 to %.1f", maxModelTemperature)
		}
		temperature = parsed
	}
	responseFormat := strings.ToLower(strings.TrimSpace(os.Getenv("OPENAI_RESPONSE_FORMAT")))
	if responseFormat == "" {
		responseFormat = defaultResponseFormat
	}
	if responseFormat != defaultResponseFormat && responseFormat != "none" {
		return nil, fmt.Errorf("OPENAI_RESPONSE_FORMAT must be json_object or none")
	}

	return newOpenAICompatibleClient(baseURL, os.Getenv("OPENAI_API_KEY"), model, temperature, responseFormat)
}
