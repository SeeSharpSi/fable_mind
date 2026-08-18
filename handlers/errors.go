package handlers

import (
	"fmt"
	"html"
	"math/rand"
	"net/http"
	"story_ai/llm"
	"story_ai/metrics"
	"story_ai/session"
	"story_ai/story"
	"story_ai/templates"
	"strings"
	"time"
)

// ErrorType represents different types of errors that can occur
type ErrorType string

const (
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeAI         ErrorType = "ai"
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeSystem     ErrorType = "system"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeTimeout    ErrorType = "timeout"
)

// UserFriendlyError represents a user-friendly error message
type UserFriendlyError struct {
	Type       ErrorType
	Message    string
	Suggestion string
	CanRetry   bool
	RetryAfter time.Duration
}

// getUserFriendlyError converts technical errors into user-friendly messages
func getUserFriendlyError(err error, errorType ErrorType) UserFriendlyError {
	errMsg := strings.ToLower(err.Error())

	switch errorType {
	case ErrorTypeAI:
		if strings.Contains(errMsg, "quota") || strings.Contains(errMsg, "limit") {
			return UserFriendlyError{
				Type:       ErrorTypeAI,
				Message:    "The story generator is currently busy with too many requests.",
				Suggestion: "Please wait a moment and try again.",
				CanRetry:   true,
				RetryAfter: 30 * time.Second,
			}
		}
		if strings.Contains(errMsg, "content") || strings.Contains(errMsg, "policy") {
			return UserFriendlyError{
				Type:       ErrorTypeAI,
				Message:    "The story generator couldn't create content for that request.",
				Suggestion: "Try rephrasing your action or choosing different words.",
				CanRetry:   true,
			}
		}
		return UserFriendlyError{
			Type:       ErrorTypeAI,
			Message:    "The story generator encountered an unexpected issue.",
			Suggestion: "Please try again in a few moments.",
			CanRetry:   true,
			RetryAfter: 10 * time.Second,
		}

	case ErrorTypeNetwork:
		return UserFriendlyError{
			Type:       ErrorTypeNetwork,
			Message:    "Unable to connect to the story generator.",
			Suggestion: "Please check your internet connection and try again.",
			CanRetry:   true,
			RetryAfter: 5 * time.Second,
		}

	case ErrorTypeTimeout:
		return UserFriendlyError{
			Type:       ErrorTypeTimeout,
			Message:    "The story is taking longer than expected to generate.",
			Suggestion: "This can happen during busy times. Please try again.",
			CanRetry:   true,
			RetryAfter: 15 * time.Second,
		}

	case ErrorTypeRateLimit:
		return UserFriendlyError{
			Type:       ErrorTypeRateLimit,
			Message:    "You're sending requests too quickly.",
			Suggestion: "Please wait a moment before trying again.",
			CanRetry:   true,
			RetryAfter: 60 * time.Second,
		}

	case ErrorTypeValidation:
		return UserFriendlyError{
			Type:       ErrorTypeValidation,
			Message:    err.Error(), // Validation errors are already user-friendly
			Suggestion: "Please check your input and try again.",
			CanRetry:   true,
		}

	default:
		return UserFriendlyError{
			Type:       ErrorTypeSystem,
			Message:    "Something unexpected happened.",
			Suggestion: "Please try again. If the problem persists, try refreshing the page.",
			CanRetry:   true,
			RetryAfter: 5 * time.Second,
		}
	}
}

// getRandomErrorResponse returns a varied error response to keep things interesting
func getRandomErrorResponse(baseMessage string) string {
	responses := []string{
		fmt.Sprintf("🤖 %s", baseMessage),
		fmt.Sprintf("📖 %s", baseMessage),
		fmt.Sprintf("✨ %s", baseMessage),
		fmt.Sprintf("🌟 %s", baseMessage),
		fmt.Sprintf("📚 %s", baseMessage),
	}

	return responses[rand.Intn(len(responses))]
}

// createErrorPage creates a user-friendly error page
func createErrorPage(userAction string, friendlyError UserFriendlyError) story.StoryPage {
	var response strings.Builder

	response.WriteString(getRandomErrorResponse(friendlyError.Message))
	response.WriteString("\n\n")
	response.WriteString(fmt.Sprintf("💡 %s", friendlyError.Suggestion))

	if friendlyError.CanRetry {
		response.WriteString("\n\n")
		if friendlyError.RetryAfter > 0 {
			response.WriteString(fmt.Sprintf("⏰ You can try again in %d seconds.", int(friendlyError.RetryAfter.Seconds())))
		} else {
			response.WriteString("🔄 Feel free to try again!")
		}
	}

	return story.StoryPage{
		Prompt:   userAction,
		Response: response.String(),
	}
}

// handleAIError handles AI-related errors without replacing current story state.
func handleAIError(w http.ResponseWriter, r *http.Request, sess *session.Session, userAction, provider string, generationStats llm.GenerationStats, err error) {
	// Record metrics
	recordGenerationMetrics(provider, generationStats, false)
	metrics.RecordError("ai_api_failure")

	friendlyError := getUserFriendlyError(err, ErrorTypeAI)
	errorPage := createErrorPage(userAction, friendlyError)

	sess.StoryHistory = append(sess.StoryHistory, errorPage)
	templates.Update(sess.StoryHistory, sess.GameState.PlayerStatus, sess.GameState.Inventory, "#1e1e1e", false, false, sess.CurrentGenre, sess.GameState.Rules.ConsequenceModel, sess.GameState.World.WorldTension, sess.CurrentAuthor).Render(r.Context(), w)
}

// handleValidationError handles validation errors
func handleValidationError(w http.ResponseWriter, r *http.Request, sess *session.Session, userAction string, err error) {
	metrics.RecordError("input_validation")

	friendlyError := getUserFriendlyError(err, ErrorTypeValidation)
	errorPage := createErrorPage(userAction, friendlyError)

	sess.StoryHistory = append(sess.StoryHistory, errorPage)
	templates.Update(sess.StoryHistory, sess.GameState.PlayerStatus, sess.GameState.Inventory, "#1e1e1e", false, false, sess.CurrentGenre, sess.GameState.Rules.ConsequenceModel, sess.GameState.World.WorldTension, sess.CurrentAuthor).Render(r.Context(), w)
}

// handleSystemError handles system-level errors
func handleSystemError(w http.ResponseWriter, r *http.Request, sess *session.Session, userAction string, err error, errorType ErrorType) {
	metrics.RecordError(string(errorType))

	friendlyError := getUserFriendlyError(err, errorType)
	errorPage := createErrorPage(userAction, friendlyError)

	sess.StoryHistory = append(sess.StoryHistory, errorPage)
	templates.Update(sess.StoryHistory, sess.GameState.PlayerStatus, sess.GameState.Inventory, "#1e1e1e", false, false, sess.CurrentGenre, sess.GameState.Rules.ConsequenceModel, sess.GameState.World.WorldTension, sess.CurrentAuthor).Render(r.Context(), w)
}

// handleStartStoryError handles errors during initial story generation
func handleStartStoryError(w http.ResponseWriter, r *http.Request, err error, errorType ErrorType) {
	friendlyError := getUserFriendlyError(err, errorType)
	safeMessage := html.EscapeString(getRandomErrorResponse(friendlyError.Message))
	safeSuggestion := html.EscapeString(friendlyError.Suggestion)

	errorHTML := fmt.Sprintf(`
		<div id="story-container">
			<h1>Story Generation Error</h1>
			<div style="background-color: #252526; padding: 20px; border-radius: 8px; margin: 20px 0;">
				<p>%s</p>
				<p><strong>Suggestion:</strong> %s</p>
				%s
			</div>
			<button onclick="window.location.reload()">Try Again</button>
		</div>
	`, safeMessage, safeSuggestion,
		func() string {
			if friendlyError.CanRetry {
				return `<p><em>You can try again by refreshing the page.</em></p>`
			}
			return ""
		}())

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(errorHTML))
}
