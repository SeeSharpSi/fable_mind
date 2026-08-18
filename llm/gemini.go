package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type geminiClient struct {
	client *genai.Client
	model  string
}

// NewGeminiClient creates a Gemini-backed client.
func NewGeminiClient(ctx context.Context, apiKey, model string) (Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("Gemini API key cannot be empty")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("Gemini model cannot be empty")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("create Gemini client: %w", err)
	}

	return &geminiClient{client: client, model: strings.TrimSpace(model)}, nil
}

func (c *geminiClient) Generate(ctx context.Context, systemInstruction, prompt string) (string, error) {
	model := c.client.GenerativeModel(c.model)
	temperature := float32(0.9)
	model.GenerationConfig = genai.GenerationConfig{
		Temperature:      &temperature,
		ResponseMIMEType: "application/json",
	}
	if systemInstruction != "" {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(systemInstruction)},
		}
	}

	response, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("Gemini generation failed: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("Gemini returned no response")
	}

	for _, candidate := range response.Candidates {
		if candidate.Content == nil {
			continue
		}

		var output strings.Builder
		for _, part := range candidate.Content.Parts {
			if text, ok := part.(genai.Text); ok {
				output.WriteString(string(text))
			}
		}
		if output.Len() > 0 {
			return output.String(), nil
		}
	}

	return "", fmt.Errorf("Gemini returned no text")
}

func (c *geminiClient) Provider() string {
	return "gemini"
}

func (c *geminiClient) Close() error {
	return c.client.Close()
}
