package handlers

import (
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeStoryHTMLAllowsRequiredMarkup(t *testing.T) {
	input := `<strong onclick="bad()">Bold</strong><br><em>quiet</em> <span class="item-added" style="color:red">key</span> <span class="proper-noun tooltip" tabindex="7" onclick="bad()">the King<span class="tooltiptext">a patient ruler</span></span>`
	got, err := sanitizeStoryHTML(input)
	if err != nil {
		t.Fatalf("sanitizeStoryHTML() error = %v", err)
	}
	want := `<strong>Bold</strong><br><em>quiet</em> <span class="item-added">key</span> <span class="proper-noun tooltip" tabindex="0">the King<span class="tooltiptext">a patient ruler</span></span>`
	if got != want {
		t.Errorf("sanitizeStoryHTML() = %q, want %q", got, want)
	}
}

func TestSanitizeStoryHTMLRemovesUnsafeMarkup(t *testing.T) {
	input := `Safe<script>alert(1)</script><style>body{display:none}</style><img src=x onerror=bad()><a href="javascript:bad()">link</a><svg><script>bad()</script></svg>`
	got, err := sanitizeStoryHTML(input)
	if err != nil {
		t.Fatalf("sanitizeStoryHTML() error = %v", err)
	}
	if got != "Safelink" {
		t.Errorf("sanitizeStoryHTML() = %q, want safe text only", got)
	}
}

func TestParseAIResponseRejectsUnsafeShapeAndColor(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{"unknown field", `{"state_patch":{},"story_update":{"story":"safe","extra":true}}`},
		{"trailing JSON", `{"state_patch":{},"story_update":{"story":"safe"}} {}`},
		{"invalid color", `{"state_patch":{},"story_update":{"story":"safe","background_color":"red;display:none"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseAIResponse(test.response, responseModeTurn); err == nil {
				t.Fatal("parseAIResponse() error = nil, want validation error")
			}
		})
	}
}

func TestParseAIResponseSanitizesModelStory(t *testing.T) {
	response, err := parseAIResponse(`{"state_patch":{},"story_update":{"story":"**Safe**<script>bad()</script>","background_color":"#aabbcc"}}`, responseModeTurn)
	if err != nil {
		t.Fatalf("parseAIResponse() error = %v", err)
	}
	if response.StoryUpdate.Story != "<strong>Safe</strong>" {
		t.Errorf("story = %q, want sanitized markup", response.StoryUpdate.Story)
	}
	if strings.Contains(response.StoryUpdate.Story, "script") {
		t.Error("sanitized story contains script markup")
	}
}

func TestParseAIResponseAcceptsCommonJSONFences(t *testing.T) {
	for _, response := range []string{
		"```json\r\n{\"state_patch\":{},\"story_update\":{\"story\":\"safe\"}}\r\n```",
		"```json {\"state_patch\":{},\"story_update\":{\"story\":\"safe\"}} ```",
	} {
		if _, err := parseAIResponse(response, responseModeTurn); err != nil {
			t.Errorf("parseAIResponse() fenced response error = %v", err)
		}
	}
}

func TestResponseValidationReasonDoesNotExposeUnknownField(t *testing.T) {
	reason := responseValidationReason(fmt.Errorf(`json: unknown field "secret-model-output"`))
	if strings.Contains(reason, "secret-model-output") {
		t.Errorf("validation reason exposes model field: %q", reason)
	}
}
