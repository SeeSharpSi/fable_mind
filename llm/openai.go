package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type openAICompatibleClient struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

type chatCompletionRequest struct {
	Model          string                  `json:"model"`
	Messages       []chatCompletionMessage `json:"messages"`
	Temperature    float64                 `json:"temperature"`
	ResponseFormat chatResponseFormat      `json:"response_format"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatCompletionMessage `json:"message"`
	} `json:"choices"`
}

// NewOpenAICompatibleClient creates a client for an OpenAI-compatible Chat Completions endpoint.
func NewOpenAICompatibleClient(baseURL, apiKey, model string) (Client, error) {
	endpoint, err := chatCompletionsEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("OpenAI-compatible model cannot be empty")
	}

	return &openAICompatibleClient{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(apiKey),
		model:    strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}, nil
}

func chatCompletionsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("parse OPENAI_BASE_URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("OPENAI_BASE_URL must be an absolute HTTP URL")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/chat/completions") {
		parsed.Path += "/chat/completions"
	}
	return parsed.String(), nil
}

func (c *openAICompatibleClient) Generate(ctx context.Context, systemInstruction, prompt string) (string, error) {
	messages := make([]chatCompletionMessage, 0, 2)
	if systemInstruction != "" {
		messages = append(messages, chatCompletionMessage{Role: "system", Content: systemInstruction})
	}
	messages = append(messages, chatCompletionMessage{Role: "user", Content: prompt})

	payload, err := json.Marshal(chatCompletionRequest{
		Model:          c.model,
		Messages:       messages,
		Temperature:    0.9,
		ResponseFormat: chatResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return "", fmt.Errorf("encode OpenAI-compatible request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create OpenAI-compatible request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("OpenAI-compatible request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if readErr != nil {
			return "", fmt.Errorf("OpenAI-compatible request failed (%s)", response.Status)
		}

		message := strings.TrimSpace(string(body))
		var apiError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &apiError) == nil && apiError.Error.Message != "" {
			message = apiError.Error.Message
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return "", fmt.Errorf("OpenAI-compatible request failed (%s): %s", response.Status, message)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 10<<20)).Decode(&completion); err != nil {
		return "", fmt.Errorf("decode OpenAI-compatible response: %w", err)
	}
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("OpenAI-compatible endpoint returned no text")
	}

	return completion.Choices[0].Message.Content, nil
}

func (c *openAICompatibleClient) Provider() string {
	return "openai-compatible"
}

func (c *openAICompatibleClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}
