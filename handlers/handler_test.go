package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"story_ai/llm"
	"story_ai/session"
	"story_ai/story"
)

type stubLLM struct {
	results []llm.Generation
	errors  []error
	prompts []string
	calls   int
}

func (s *stubLLM) Generate(_ context.Context, _, prompt string, _ llm.GenerationOptions) (llm.Generation, error) {
	if s.calls >= len(s.results) {
		return llm.Generation{}, fmt.Errorf("unexpected Generate call %d", s.calls+1)
	}
	s.prompts = append(s.prompts, prompt)
	result := s.results[s.calls]
	var err error
	if s.calls < len(s.errors) {
		err = s.errors[s.calls]
	}
	s.calls++
	return result, err
}

func (s *stubLLM) Provider() string { return "stub" }
func (s *stubLLM) Close() error     { return nil }

func TestParseAndRetryAIResponseAggregatesStats(t *testing.T) {
	retry := llm.Generation{
		Text: `{"new_game_state":{},"story_update":{"story":"continued"}}`,
		Stats: llm.GenerationStats{
			Usage:              llm.TokenUsage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			Duration:           2 * time.Second,
			InputCharacters:    80,
			OutputCharacters:   40,
			Requests:           1,
			ResponsesWithUsage: 1,
		},
	}
	client := &stubLLM{results: []llm.Generation{retry}}
	handler := &Handler{LLM: client}
	original := llm.Generation{
		Text: "not json",
		Stats: llm.GenerationStats{
			Usage:              llm.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			Duration:           time.Second,
			InputCharacters:    40,
			OutputCharacters:   8,
			Requests:           1,
			ResponsesWithUsage: 1,
		},
	}

	response, stats, err := handler.parseAndRetryAIResponse(context.Background(), "system", `{"mode":"start"}`, responseModeStart, llm.GenerationOptions{MaxOutputTokens: 1000}, nil, original)
	if err != nil {
		t.Fatalf("parseAndRetryAIResponse() error = %v", err)
	}
	if response.StoryUpdate.Story != "continued" {
		t.Errorf("story = %q, want continued", response.StoryUpdate.Story)
	}
	if client.calls != 1 || stats.Requests != 2 {
		t.Errorf("calls = %d, requests = %d; want 1 retry and 2 total requests", client.calls, stats.Requests)
	}
	if stats.Usage != (llm.TokenUsage{PromptTokens: 30, CompletionTokens: 15, TotalTokens: 45}) {
		t.Errorf("usage = %#v, want aggregate usage", stats.Usage)
	}
	if stats.Duration != 3*time.Second || stats.InputCharacters != 120 || stats.OutputCharacters != 48 {
		t.Errorf("stats = %#v, want aggregate duration and characters", stats)
	}
}

func TestParseAndRetryAIResponseRetriesSemanticFailureOnce(t *testing.T) {
	client := &stubLLM{results: []llm.Generation{{
		Text:  `{"state_patch":{},"story_update":{"story":"fixed"}}`,
		Stats: llm.GenerationStats{Requests: 1},
	}}}
	handler := &Handler{LLM: client}
	original := llm.Generation{Text: `{"state_patch":{},"story_update":{"story":"wrong response"}}`, Stats: llm.GenerationStats{Requests: 1}}
	validator := func(response AIResponse) error {
		if response.StoryUpdate.Story != "fixed" {
			return errors.New("semantic failure")
		}
		return nil
	}

	response, stats, err := handler.parseAndRetryAIResponse(context.Background(), "system", `{"mode":"turn","user_action":"wait"}`, responseModeTurn, llm.GenerationOptions{}, validator, original)
	if err != nil {
		t.Fatalf("parseAndRetryAIResponse() error = %v", err)
	}
	if response.StoryUpdate.Story != "fixed" || client.calls != 1 || stats.Requests != 2 {
		t.Errorf("response/calls/stats = %#v/%d/%#v", response, client.calls, stats)
	}
	if strings.Contains(client.prompts[0], "wrong response") {
		t.Error("retry prompt contains invalid model output")
	}
	if !strings.Contains(client.prompts[0], "state transition failed integrity checks") || !strings.Contains(client.prompts[0], `"user_action":"wait"`) {
		t.Errorf("retry prompt missing compact error or original request: %q", client.prompts[0])
	}
}

func TestParseAndRetryAIResponseStopsAfterOneRetry(t *testing.T) {
	client := &stubLLM{results: []llm.Generation{{Text: "still invalid", Stats: llm.GenerationStats{Requests: 1}}}}
	handler := &Handler{LLM: client}
	original := llm.Generation{Text: "invalid", Stats: llm.GenerationStats{Requests: 1}}

	_, stats, err := handler.parseAndRetryAIResponse(context.Background(), "system", `{}`, responseModeTurn, llm.GenerationOptions{}, nil, original)
	if err == nil {
		t.Fatal("parseAndRetryAIResponse() error = nil, want final validation error")
	}
	if client.calls != 1 || stats.Requests != 2 {
		t.Errorf("calls = %d, requests = %d; want exactly one retry", client.calls, stats.Requests)
	}
}

func TestGenerationOptions(t *testing.T) {
	t.Setenv("TEST_MAX_TOKENS", "1500")
	if got := generationOptions("TEST_MAX_TOKENS", 900).MaxOutputTokens; got != 1500 {
		t.Errorf("configured max output tokens = %d, want 1500", got)
	}

	t.Setenv("TEST_MAX_TOKENS", "invalid")
	if got := generationOptions("TEST_MAX_TOKENS", 900).MaxOutputTokens; got != 900 {
		t.Errorf("fallback max output tokens = %d, want 900", got)
	}
}

func TestParseAIResponseEnforcesModeContract(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		response string
		wantErr  bool
	}{
		{
			name:     "start accepts full state",
			mode:     responseModeStart,
			response: `{"new_game_state":{},"story_update":{"story":"begin"}}`,
		},
		{
			name:     "start rejects patch",
			mode:     responseModeStart,
			response: `{"state_patch":{},"story_update":{"story":"begin"}}`,
			wantErr:  true,
		},
		{
			name:     "turn accepts empty patch",
			mode:     responseModeTurn,
			response: `{"state_patch":{},"story_update":{"story":"wait"}}`,
		},
		{
			name:     "turn rejects full state",
			mode:     responseModeTurn,
			response: `{"new_game_state":{},"story_update":{"story":"wait"}}`,
			wantErr:  true,
		},
		{
			name:     "rejects empty story",
			mode:     responseModeTurn,
			response: `{"state_patch":{},"story_update":{"story":""}}`,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseAIResponse(test.response, test.mode)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseAIResponse() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestGenerateRejectsCompletedStoryWithoutModelCall(t *testing.T) {
	manager := session.NewManager()
	id := manager.CreateSession()
	sess := manager.GetSession(id)
	sess.GameState = &story.GameState{
		PlayerStatus: story.PlayerStatus{Health: 50},
		Environment:  story.Environment{LocationName: "Hall"},
		Rules:        story.Rules{ConsequenceModel: "challenging"},
		GameWon:      true,
	}
	client := &stubLLM{}
	handler := &Handler{LLM: client, Manager: manager}
	form := url.Values{"prompt": {"wait"}}
	request := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "session_id", Value: id})
	recorder := httptest.NewRecorder()

	handler.Generate(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if client.calls != 0 {
		t.Errorf("model calls = %d, want 0", client.calls)
	}
}

func TestHandleAIErrorPreservesStoryState(t *testing.T) {
	gameState := &story.GameState{
		PlayerStatus: story.PlayerStatus{Health: 75},
		Rules:        story.Rules{ConsequenceModel: "challenging"},
	}
	sess := &session.Session{GameState: gameState}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/generate", nil)

	handleAIError(recorder, request, sess, "open door", "stub", llm.GenerationStats{}, errors.New("timeout"))
	if sess.GameState != gameState || sess.GameState.PlayerStatus.Health != 75 {
		t.Errorf("game state changed after API error: %#v", sess.GameState)
	}
	if len(sess.StoryHistory) != 1 {
		t.Errorf("story history length = %d, want one error page", len(sess.StoryHistory))
	}
}

func TestStartStoryFailureRestoresSession(t *testing.T) {
	manager := session.NewManager()
	id := manager.CreateSession()
	sess := manager.GetSession(id)
	previousState := &story.GameState{
		PlayerStatus: story.PlayerStatus{Health: 60},
		Environment:  story.Environment{LocationName: "Old Keep"},
		Rules:        story.Rules{ConsequenceModel: "exploratory"},
	}
	sess.GameState = previousState
	sess.StoryHistory = []story.StoryPage{{Prompt: "old", Response: "old story"}}
	sess.CurrentGenre = "historical-fiction"
	sess.CurrentAuthor = "Old Author"
	sess.NarratorPersona = "historian"
	client := &stubLLM{
		results: []llm.Generation{{Stats: llm.GenerationStats{Requests: 1}}},
		errors:  []error{errors.New("model unavailable")},
	}
	handler := &Handler{LLM: client, Manager: manager, DataDatabasePath: "../data.db"}
	request := httptest.NewRequest(http.MethodGet, "/start?genre=fantasy&consequence_model=punishing", nil)
	request.AddCookie(&http.Cookie{Name: "session_id", Value: id})
	recorder := httptest.NewRecorder()

	handler.StartStory(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if client.calls != 1 {
		t.Fatalf("model calls = %d, want 1", client.calls)
	}
	if sess.GameState != previousState || sess.CurrentGenre != "historical-fiction" || sess.CurrentAuthor != "Old Author" || sess.NarratorPersona != "historian" {
		t.Errorf("session metadata changed after failed start: %#v", sess)
	}
	if len(sess.StoryHistory) != 1 || sess.StoryHistory[0].Response != "old story" {
		t.Errorf("story history changed after failed start: %#v", sess.StoryHistory)
	}
}

func TestGenerateSendsFilteredModelContext(t *testing.T) {
	manager := session.NewManager()
	id := manager.CreateSession()
	sess := manager.GetSession(id)
	nouns := make([]story.ProperNoun, 12)
	for i := range nouns {
		nouns[i] = story.ProperNoun{
			Noun:        fmt.Sprintf("Person %d", i),
			PhraseUsed:  fmt.Sprintf("figure %d", i),
			Description: "a remembered traveler",
		}
	}
	sess.GameState = &story.GameState{
		PlayerStatus:   story.PlayerStatus{Health: 100, Stamina: 100, Conditions: []string{}},
		Environment:    story.Environment{LocationName: "Square", Description: "A quiet square", Exits: map[string]string{}},
		Rules:          story.Rules{ConsequenceModel: "challenging"},
		WinConditions:  []string{"find the route"},
		LossConditions: []string{"miss the departure"},
		ProperNouns:    nouns,
	}
	sess.CurrentGenre = "fantasy"
	sess.CurrentAuthor = "Test Author"
	client := &stubLLM{results: []llm.Generation{{Text: `{"state_patch":{},"story_update":{"story":"You wait in the square.","background_color":"#223344"}}`}}}
	handler := &Handler{LLM: client, Manager: manager}
	form := url.Values{"prompt": {"Ask Person 0 for help"}}
	request := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "session_id", Value: id})
	recorder := httptest.NewRecorder()

	handler.Generate(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var modelRequest AIRequest
	if err := json.Unmarshal([]byte(client.prompts[0]), &modelRequest); err != nil {
		t.Fatalf("decode model request: %v", err)
	}
	if len(modelRequest.GameState.ProperNouns) != 9 {
		t.Errorf("model noun count = %d, want referenced noun plus 8 recent nouns", len(modelRequest.GameState.ProperNouns))
	}
	if len(sess.GameState.ProperNouns) != 12 {
		t.Errorf("persisted glossary length = %d, want 12", len(sess.GameState.ProperNouns))
	}
}

func TestHandleStartStoryErrorEscapesValidationMessage(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/start", nil)
	handleStartStoryError(recorder, request, errors.New(`<img src=x onerror=alert(1)>`), ErrorTypeValidation)

	body := recorder.Body.String()
	if strings.Contains(body, `<img`) || !strings.Contains(body, `&lt;img`) {
		t.Errorf("response contains unescaped validation message: %q", body)
	}
}

func TestConfiguredStatsServiceURL(t *testing.T) {
	t.Setenv("STATS_SERVICE_URL", " https://stats.example.test/ ")
	if got := configuredStatsServiceURL(); got != "https://stats.example.test" {
		t.Fatalf("configured URL = %q, want trimmed URL", got)
	}

	t.Setenv("STATS_SERVICE_URL", "")
	if got := configuredStatsServiceURL(); got != "" {
		t.Fatalf("unset configured URL = %q, want empty URL", got)
	}
}
