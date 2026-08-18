package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"slices"
	"story_ai/llm"
	"story_ai/metrics"
	"story_ai/prompts"
	"story_ai/session"
	"story_ai/story"
	"story_ai/templates"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/jung-kurt/gofpdf"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	_ "modernc.org/sqlite"
)

type NarratorOption struct {
	Name string
	// A function to set the specific flag on the session
	ApplyFlag func(s *session.Session)
	// Whitelist of genres (empty = all allowed)
	AllowedGenres []string
	// Blacklist of genres (empty = none excluded)
	ExcludedGenres []string
	// Relative chance to be picked (higher = more likely)
	Weight int
}

type Handler struct {
	LLM     llm.Client
	Manager *session.Manager
}

// AIResponse is the top-level structure for the AI's JSON response.
type AIResponse struct {
	NewGameState *story.GameState `json:"new_game_state"`
	StoryUpdate  StoryUpdate      `json:"story_update"`
}

// StoryUpdate contains the narrative portion of the AI's response.
type StoryUpdate struct {
	Story           string   `json:"story"`
	ItemsAdded      []string `json:"items_added"`
	ItemsRemoved    []string `json:"items_removed"`
	GameOver        bool     `json:"game_over"`
	BackgroundColor string   `json:"background_color"`
}

// AIRequest is the structure sent to the AI.
type AIRequest struct {
	GameState  *story.GameState `json:"game_state"`
	UserAction string           `json:"user_action"`
}

var (
	authors = []string{"James Joyce", "Mark Twain", "Jack Kerouac", "Kurt Vonnegut", "H.P. Lovecraft", "Edgar Allan Poe", "J.R.R. Tolkien", "Terry Pratchett"}
	// Regex to find Markdown bolding (**text**)
	markdownBoldRegex = regexp.MustCompile(`\*\*(.*?)\*\*`)
	// Regex to find Markdown italics (*text*)
	markdownItalicRegex = regexp.MustCompile(`\*(.*?)\*`)
	// Regex to fix punctuation outside of span tags
	spanPunctuationRegex = regexp.MustCompile(`(<span\s+class="[^"]*">(?:.|\n)*?)(</span>)([.,?!])`)
)

// validateUserAction validates user input for security and appropriateness
func validateUserAction(action string) error {
	// Check length constraints
	if len(action) == 0 {
		return fmt.Errorf("action cannot be empty")
	}
	if len(action) > 500 {
		return fmt.Errorf("action must be 500 characters or less")
	}
	if len(strings.Fields(action)) > 15 {
		return fmt.Errorf("action must be 15 words or less")
	}

	// Check for potentially harmful content
	dangerousPatterns := []string{
		`<script`, `javascript:`, `on\w+\s*=`, `<iframe`, `<object`, `<embed`,
		`eval\s*\(`, `document\.`, `window\.`, `alert\s*\(`, `prompt\s*\(`,
		`confirm\s*\(`, `setTimeout\s*\(`, `setInterval\s*\(`,
	}

	actionLower := strings.ToLower(action)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(actionLower, pattern) {
			return fmt.Errorf("action contains potentially harmful content")
		}
	}

	// Check for excessive special characters
	specialChars := 0
	for _, char := range action {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && !unicode.IsSpace(char) && !unicode.IsPunct(char) {
			specialChars++
		}
	}
	if specialChars > len(action)/10 { // More than 10% special characters
		return fmt.Errorf("action contains too many special characters")
	}

	return nil
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

// parseAIResponse unmarshals the JSON from the AI and cleans up the story text.
func parseAIResponse(response string) (AIResponse, error) {
	var aiResp AIResponse
	cleanResponse := strings.TrimPrefix(response, "```json\n")
	cleanResponse = strings.TrimSuffix(cleanResponse, "\n```")

	err := json.Unmarshal([]byte(cleanResponse), &aiResp)
	if err != nil {
		return aiResp, err
	}

	story := aiResp.StoryUpdate.Story
	story = markdownBoldRegex.ReplaceAllString(story, "<strong>$1</strong>")
	story = markdownItalicRegex.ReplaceAllString(story, "<em>$1</em>")
	story = spanPunctuationRegex.ReplaceAllString(story, "$1$3$2")

	aiResp.StoryUpdate.Story = story

	return aiResp, nil
}

func (h *Handler) parseAndRetryAIResponse(ctx context.Context, systemInstruction, originalResponse string) (AIResponse, error) {
	log.Printf("RAW AI RESPONSE: %s", originalResponse)
	aiResp, err := parseAIResponse(originalResponse)
	if err == nil {
		return aiResp, nil
	}

	log.Printf("Initial JSON parsing failed: %v. Retrying with the AI.", err)

	for i := range 3 { // Retry up to 3 times
		retryPrompt := fmt.Sprintf(prompts.JsonRetryPrompt, originalResponse)
		correctedResponse, retryErr := h.LLM.Generate(ctx, systemInstruction, retryPrompt)
		if retryErr != nil {
			log.Printf("AI retry attempt %d failed: %v", i+1, retryErr)
			continue
		}

		aiResp, err = parseAIResponse(correctedResponse)
		if err == nil {
			log.Printf("AI successfully corrected the JSON on attempt %d.", i+1)
			return aiResp, nil
		}
		log.Printf("AI retry attempt %d still resulted in invalid JSON: %v", i+1, err)
		originalResponse = correctedResponse // Use the corrected (but still invalid) response for the next retry
	}

	return AIResponse{}, fmt.Errorf("failed to parse AI response after multiple retries")
}

func prettyPrint(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error pretty printing: %v", err)
	}
	return string(b)
}

func (h *Handler) buildSystemPrompt(s *session.Session) string {
	// Start with the base prompt, injecting the current author's name
	prompt := fmt.Sprintf(prompts.BasePrompt, s.CurrentAuthor)

	// Append the specific persona prompt based on the session's NarratorPersona ID
	switch s.NarratorPersona {
	case "funny":
		prompt += prompts.FunnyStoryPrompt
	case "angry":
		prompt += prompts.AngryPrompt
	case "xkcd":
		prompt += prompts.XKCDPrompt
	case "stanley":
		prompt += prompts.StanleyPrompt
	case "glados":
		prompt += prompts.GLaDOSPrompt
	case "kreia":
		prompt += prompts.KreiaPrompt
	case "nietzsche":
		prompt += prompts.NietzschePrompt
	case "bunyan":
		prompt += prompts.BunyanPrompt
	case "socrates":
		prompt += prompts.SocraticPrompt
	case "historian":
		prompt += prompts.HistorianPrompt
	case "ross_ramsay":
		prompt += prompts.RossRamsayPrompt
	case "snoop_child":
		prompt += prompts.SnoopChildPrompt
	case "dr_seuss":
		prompt += prompts.DrSeussPrompt
	case "tolstoy_camus":
		prompt += prompts.TolstoyVsCamusPrompt
	case "bastion":
		prompt += prompts.BastionPrompt
	case "diogenes_chesterton":
		prompt += prompts.DiogenesVsChestertonPrompt
	case "thompson":
		prompt += prompts.ThompsonPrompt
	case "fishburne":
		prompt += prompts.FishburnePrompt
	case "blanchett":
		prompt += prompts.BlanchettPrompt
	}

	// Append the genre-specific prompt
	switch s.CurrentGenre {
	case "fantasy":
		prompt += prompts.FantasyPrompt
	case "sci-fi":
		prompt += prompts.SciFiPrompt
	case "historical-fiction":
		// Historical fiction requires injecting specific event details
		prompt += fmt.Sprintf(prompts.HistoricalFictionPrompt, s.HistoricalEvent, s.HistoricalDesc, s.HistoricalSummary)
	default:
		prompt += prompts.FantasyPrompt
	}

	return prompt
}

func (h *Handler) StartStory(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	sess, cookie := h.Manager.GetOrCreateSession(r)
	http.SetCookie(w, &cookie)

	genre := r.URL.Query().Get("genre")
	consequenceModel := r.URL.Query().Get("consequence_model")

	// Validate genre parameter
	validGenres := []string{"fantasy", "sci-fi", "historical-fiction"}
	if genre != "" && !contains(validGenres, genre) {
		metrics.RecordStoryGeneration(time.Since(startTime), genre, consequenceModel, false)
		err := fmt.Errorf("invalid genre parameter: %s", genre)
		handleStartStoryError(w, r, err, ErrorTypeValidation)
		return
	}

	// Validate consequence model parameter
	validModels := []string{"exploratory", "challenging", "punishing"}
	if consequenceModel != "" && !contains(validModels, consequenceModel) {
		metrics.RecordStoryGeneration(time.Since(startTime), genre, consequenceModel, false)
		err := fmt.Errorf("invalid consequence_model parameter: %s", consequenceModel)
		handleStartStoryError(w, r, err, ErrorTypeValidation)
		return
	}

	sess.GameState.Rules.ConsequenceModel = consequenceModel
	sess.CurrentGenre = genre

	// Reset story history for a new game
	sess.StoryHistory = []story.StoryPage{}
	sess.NarratorPersona = ""

	author := h.pickNarrator(sess, genre)

	sess.CurrentAuthor = author
	log.Printf("--- NEW STORY --- Author: %s, Genre: %s, Difficulty: %s", author, genre, consequenceModel)
	go pingStatsService("start", nil)

	var prompt string
	var inspirationTitle, inspirationDesc string

	db, err := sql.Open("sqlite", "./data.db")
	if err != nil {
		http.Error(w, "Failed to open database.", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	switch genre {
	case "fantasy":
		err = db.QueryRow("SELECT title, description FROM fantasy_inspo ORDER BY RANDOM() LIMIT 1").Scan(&inspirationTitle, &inspirationDesc)
		if err != nil {
			http.Error(w, "Failed to query database for fantasy inspiration.", http.StatusInternalServerError)
			return
		}
		prompt = h.buildSystemPrompt(sess)
		prompt += fmt.Sprintf("\n- You MUST use the following title and description as inspiration for the story:\n- Title: %s\n- Description: %s\n", inspirationTitle, inspirationDesc)
	case "sci-fi":
		err = db.QueryRow("SELECT title, description FROM scifi_inspo ORDER BY RANDOM() LIMIT 1").Scan(&inspirationTitle, &inspirationDesc)
		if err != nil {
			http.Error(w, "Failed to query database for sci-fi inspiration.", http.StatusInternalServerError)
			return
		}
		prompt = h.buildSystemPrompt(sess)
		prompt += fmt.Sprintf("\n- You MUST use the following title and description as inspiration for the story:\n- Title: %s\n- Description: %s\n", inspirationTitle, inspirationDesc)
	case "historical-fiction":
		var wikipediaURL string
		err = db.QueryRow("SELECT event, description, wikipedia, summary FROM historical_events ORDER BY RANDOM() LIMIT 1").Scan(&sess.HistoricalEvent, &sess.HistoricalDesc, &wikipediaURL, &sess.HistoricalSummary)
		if err != nil {
			http.Error(w, "Failed to query database for historical event.", http.StatusInternalServerError)
			return
		}
		sess.HistoricalURL = wikipediaURL
		prompt = h.buildSystemPrompt(sess)
		log.Printf("--- HISTORICAL EVENT --- Event: %s, Description: %s", sess.HistoricalEvent, sess.HistoricalDesc)
	default:
		sess.CurrentGenre = "fantasy" // Default to fantasy
		prompt = h.buildSystemPrompt(sess)
	}

	initialRequest := AIRequest{
		GameState: &story.GameState{
			PlayerStatus:      story.PlayerStatus{Health: 100, Stamina: 100, Conditions: make([]string, 0)},
			Inventory:         make([]story.Item, 0),
			Environment:       story.Environment{Exits: make(map[string]string), WorldObjects: make([]story.WorldObject, 0)},
			NPCs:              make([]story.NPC, 0),
			Puzzles:           make([]story.Puzzle, 0),
			ProperNouns:       make([]story.ProperNoun, 0),
			Rules:             story.Rules{ConsequenceModel: consequenceModel},
			World:             story.World{WorldTension: 0},
			Climax:            false,
			WinConditions:     make([]string, 0),
			LossConditions:    make([]string, 0),
			SolvedPuzzleTypes: make([]string, 0),
		},
		UserAction: "Start the game.",
	}
	reqBytes, err := json.Marshal(initialRequest)
	if err != nil {
		http.Error(w, "Failed to create initial AI request.", http.StatusInternalServerError)
		return
	}

	response, err := h.LLM.Generate(context.Background(), prompt, string(reqBytes))
	if err != nil {
		log.Printf("AI ERROR (StartStory): %v", err)
		http.Error(w, "The AI failed to start the story. Please try again.", http.StatusInternalServerError)
		return
	}

	aiResp, err := h.parseAndRetryAIResponse(context.Background(), prompt, response)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse AI's initial response: %v", err), http.StatusInternalServerError)
		return
	}

	// log.Printf("--- NEW GAME STATE (START) --- %s", prettyPrint(aiResp.NewGameState))

	if aiResp.StoryUpdate.BackgroundColor == "" {
		aiResp.StoryUpdate.BackgroundColor = "#1e1e1e"
	}

	sess.GameState = aiResp.NewGameState
	// The FoundItems list will be empty on start, so no need to update it yet.
	storyText := aiResp.StoryUpdate.Story
	if sess.NarratorPersona == "stanley" && !strings.HasPrefix(storyText, "This is the story of a man named Stanley.") {
		storyText = "This is the story of a man named Stanley.<br><br>" + storyText
	}
	sess.StoryHistory = []story.StoryPage{{Prompt: "Start", Response: storyText}}

	placeholder := "What do you do?"
	switch sess.NarratorPersona {
	case "stanley":
		placeholder = "What does Stanley do?"
	case "dr_seuss":
		placeholder = "Now what will you do?"
	case "tolstoy_camus":
		placeholder = "What is the logical choice?"
	case "bastion":
		placeholder = "What does the Kid do?"
	case "thompson":
		placeholder = "What do I do?"
	}

	templates.StoryView(storyText, aiResp.NewGameState.PlayerStatus, aiResp.NewGameState.Inventory, aiResp.StoryUpdate.BackgroundColor, genre, aiResp.NewGameState.World.WorldTension, consequenceModel, placeholder).Render(context.Background(), w)

	// Record successful story generation metrics
	metrics.RecordStoryGeneration(time.Since(startTime), genre, consequenceModel, true)
}

// pickNarrator selects a narrator persona based on the genre and weighted probabilities.
// It sets the session's NarratorPersona field and returns the display name of the author.
func (h *Handler) pickNarrator(sess *session.Session, genre string) string {
	type NarratorOption struct {
		DisplayName    string   // The name displayed to the user (e.g., "GLaDOS")
		PersonaID      string   // The ID used in buildSystemPrompt (e.g., "glados")
		AllowedGenres  []string // If set, only available for these genres
		ExcludedGenres []string // If set, not available for these genres
		Weight         int      // Probability weight
	}

	options := []NarratorOption{
		{
			DisplayName: "a very angry narrator",
			PersonaID:   "angry",
			Weight:      10,
		},
		{
			DisplayName: "the Monty Python group",
			PersonaID:   "funny",
			Weight:      10,
		},
		{
			DisplayName:   "XKCD",
			PersonaID:     "xkcd",
			AllowedGenres: []string{"sci-fi"},
			Weight:        10,
		},
		{
			DisplayName:   "Hunter S. Thompson",
			PersonaID:     "thompson",
			AllowedGenres: []string{"historical-fiction"},
			Weight:        10,
		},
		{
			DisplayName:   "Cate Blanchett",
			PersonaID:     "blanchett",
			AllowedGenres: []string{"fantasy"},
			Weight:        10,
		},
		{
			DisplayName:   "GLaDOS from Portal 2",
			PersonaID:     "glados",
			AllowedGenres: []string{"sci-fi"},
			Weight:        10,
		},
		{
			DisplayName:   "Kreia from Knights of the Old Republic II",
			PersonaID:     "kreia",
			AllowedGenres: []string{"fantasy"},
			Weight:        10,
		},
		{
			DisplayName:   "The Historian",
			PersonaID:     "historian",
			AllowedGenres: []string{"historical-fiction"},
			Weight:        10,
		},
		{
			DisplayName: "Friedrich Nietzsche",
			PersonaID:   "nietzsche",
			Weight:      10,
		},
		{
			DisplayName: "Socrates",
			PersonaID:   "socrates",
			Weight:      10,
		},
		{
			DisplayName: "Ross & Ramsay",
			PersonaID:   "ross_ramsay",
			Weight:      5,
		},
		{
			DisplayName:    "Snoop Dog & Julia Child",
			PersonaID:      "snoop_child",
			ExcludedGenres: []string{"historical-fiction"},
			Weight:         5,
		},
		{
			DisplayName:    "Dr. Seuss",
			PersonaID:      "dr_seuss",
			ExcludedGenres: []string{"historical-fiction"},
			Weight:         5,
		},
		{
			DisplayName: "Diogenes & Chesterton",
			PersonaID:   "diogenes_chesterton",
			Weight:      5,
		},
		{
			DisplayName: "Tolstoy & Camus",
			PersonaID:   "tolstoy_camus",
			Weight:      5,
		},
		{
			DisplayName: "John Bunyan",
			PersonaID:   "bunyan",
			Weight:      10,
		},
		{
			DisplayName: "the videogame Bastion",
			PersonaID:   "bastion",
			Weight:      10,
		},
		{
			DisplayName: "The Stanley Parable",
			PersonaID:   "stanley",
			Weight:      10,
		},
		{
			DisplayName: "Laurence Fishburne",
			PersonaID:   "fishburne",
			Weight:      10,
		},
		{
			DisplayName: "Standard Classic Author", // Special case
			PersonaID:   "",                        // Empty ID means default behavior
			Weight:      20,
		},
	}

	// 1. Filter valid options for the current genre
	var validOptions []NarratorOption
	totalWeight := 0

	for _, opt := range options {
		// Check AllowedGenres (whitelist)
		if len(opt.AllowedGenres) > 0 && !contains(opt.AllowedGenres, genre) {
			continue
		}
		// Check ExcludedGenres (blacklist)
		if len(opt.ExcludedGenres) > 0 && contains(opt.ExcludedGenres, genre) {
			continue
		}

		validOptions = append(validOptions, opt)
		totalWeight += opt.Weight
	}

	// 2. Weighted Random Selection
	r := rand.Intn(totalWeight)
	currentWeight := 0

	for _, opt := range validOptions {
		currentWeight += opt.Weight
		if r < currentWeight {
			// Update the session with the Persona ID
			sess.NarratorPersona = opt.PersonaID

			// If it's the special "Standard Classic Author" case, pick a random name from the global authors list
			if opt.DisplayName == "Standard Classic Author" {
				// 'authors' is the package-level variable defined in handler.go
				return authors[rand.Intn(len(authors))]
			}

			return opt.DisplayName
		}
	}

	// Fallback (should rarely be reached)
	sess.NarratorPersona = ""
	return authors[0]
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	sess, _ := h.Manager.GetOrCreateSession(r)
	userAction := r.FormValue("prompt")

	// Input validation
	if err := validateUserAction(userAction); err != nil {
		handleValidationError(w, r, sess, userAction, err)
		return
	}

	if strings.ToLower(strings.TrimSpace(userAction)) == "restart" {
		query := "genre=" + sess.CurrentGenre + "&consequence_model=" + sess.GameState.Rules.ConsequenceModel
		r.URL.RawQuery = query
		h.StartStory(w, r)
		return
	}

	if len(strings.Fields(userAction)) > 15 {
		http.Error(w, "Response must be 15 words or less.", http.StatusBadRequest)
		return
	}

	systemPrompt := h.buildSystemPrompt(sess)

	aiRequest := AIRequest{
		GameState:  sess.GameState,
		UserAction: userAction,
	}
	reqBytes, err := json.Marshal(aiRequest)
	if err != nil {
		http.Error(w, "Failed to create AI request.", http.StatusInternalServerError)
		return
	}

	response, err := h.LLM.Generate(r.Context(), systemPrompt, string(reqBytes))
	if err != nil {
		handleAIError(w, r, sess, userAction, h.LLM.Provider(), err, startTime)
		return
	}

	aiResp, err := h.parseAndRetryAIResponse(r.Context(), systemPrompt, response)
	if err != nil {
		handleSystemError(w, r, sess, userAction, err, ErrorTypeAI)
		return
	}

	// log.Printf("--- NEW GAME STATE (GENERATE) --- %s", prettyPrint(aiResp.NewGameState))

	if aiResp.StoryUpdate.BackgroundColor == "" {
		aiResp.StoryUpdate.BackgroundColor = "#1e1e1e"
	}

	if aiResp.StoryUpdate.GameOver || aiResp.NewGameState.GameWon {
		go pingStatsService("complete", nil)
	}

	// Merge the AI's proper nouns into the session's master list.
	existingNouns := make(map[string]bool)
	for _, noun := range sess.GameState.ProperNouns {
		existingNouns[noun.Noun] = true
	}
	for _, newNoun := range aiResp.NewGameState.ProperNouns {
		if !existingNouns[newNoun.Noun] {
			sess.GameState.ProperNouns = append(sess.GameState.ProperNouns, newNoun)
			existingNouns[newNoun.Noun] = true
		}
	}

	// Update the rest of the game state, but preserve our master noun list.
	updatedNouns := sess.GameState.ProperNouns
	sess.GameState = aiResp.NewGameState
	sess.GameState.ProperNouns = updatedNouns

	storyText := aiResp.StoryUpdate.Story // Use nouns from this turn for tooltips
	sess.StoryHistory = append(sess.StoryHistory, story.StoryPage{Prompt: userAction, Response: storyText})

	// Record successful AI API usage and user activity metrics
	metrics.RecordAPIUsage(h.LLM.Provider(), 0, time.Since(startTime), true) // Token count would need to be extracted from AI response
	metrics.RecordUserActivity("generate_response", sess.CurrentGenre, time.Since(startTime))

	templates.Update(sess.StoryHistory, sess.GameState.PlayerStatus, sess.GameState.Inventory, aiResp.StoryUpdate.BackgroundColor, aiResp.StoryUpdate.GameOver, sess.GameState.GameWon, sess.CurrentGenre, sess.GameState.Rules.ConsequenceModel, sess.GameState.World.WorldTension, sess.CurrentAuthor).Render(context.Background(), w)
}

// writeHtmlToPdf parses a simple HTML string and writes it to the PDF, handling nested styles.
func writeHtmlToPdf(pdf *gofpdf.Fpdf, htmlStr string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<body>" + htmlStr + "</body>"))
	if err != nil {
		pdf.MultiCell(0, 6, htmlStr, "", "", false)
		return
	}

	var currentStyle string
	var process func(*goquery.Selection)
	process = func(s *goquery.Selection) {
		s.Contents().Each(func(i int, content *goquery.Selection) {
			// Skip tooltiptext spans entirely in the main processing loop
			if content.HasClass("tooltiptext") {
				return
			}

			if goquery.NodeName(content) == "br" {
				pdf.Ln(6)
				return
			}

			if goquery.NodeName(content) == "#text" {
				pdf.SetFontStyle(currentStyle)
				pdf.Write(6, content.Text())
			} else {
				// Store parent's state
				r, g, b := pdf.GetTextColor()
				parentStyle := currentStyle

				// Determine new style for this node's children
				switch goquery.NodeName(content) {
				case "strong":
					if !strings.Contains(currentStyle, "B") {
						currentStyle += "B"
					}
				case "em":
					if !strings.Contains(currentStyle, "I") {
						currentStyle += "I"
					}
				case "span":
					if content.HasClass("item-added") {
						pdf.SetTextColor(34, 139, 34) // ForestGreen
					} else if content.HasClass("item-removed") {
						pdf.SetTextColor(165, 42, 42) // Brown
						if !strings.Contains(currentStyle, "S") {
							currentStyle += "S"
						}
					} else if content.HasClass("proper-noun") {
						// Proper nouns are bolded in the PDF instead of colored
						if !strings.Contains(currentStyle, "B") {
							currentStyle += "B"
						}
					}
				}

				// Process children with the new style
				process(content)

				// Restore parent's state for subsequent siblings
				pdf.SetTextColor(r, g, b)
				currentStyle = parentStyle
				pdf.SetFontStyle(currentStyle)
			}
		})
	}
	process(doc.Find("body"))
	pdf.SetFontStyle("") // Final reset
}

func (h *Handler) DownloadStory(w http.ResponseWriter, r *http.Request) {
	sess, _ := h.Manager.GetOrCreateSession(r)

	pdf := gofpdf.New("P", "mm", "A4", "")

	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Times", "I", 8)
		pdf.CellFormat(0, 10, fmt.Sprintf("Page %d", pdf.PageNo()), "", 0, "C", false, 0, "")
	})

	// Title Page
	pdf.AddPage()
	pdf.SetFont("Times", "B", 36)
	pdf.Cell(0, 80, "")
	pdf.Ln(-1)

	// Determine the PDF title based on the persona
	title := "Your Story"
	switch sess.NarratorPersona {
	case "funny":
		title = "A Decently Amusing Story"
	case "angry":
		title = "The Tale I Was Forced to Tell"
	case "xkcd":
		title = "Hypothesis: A Story"
	case "stanley":
		title = "The Story of a Man Named Stanley"
	case "glados":
		title = "A Mandatory Enrichment Activity"
	case "kreia":
		title = "A Lesson in Consequences"
	case "nietzsche":
		title = "Thus Spoke the Traveler"
	case "bunyan":
		title = "The Pilgrim's Burden"
	case "socrates":
		title = "An Unexamined Life"
	case "historian":
		title = "The Human Thing"
	case "ross_ramsay":
		title = "The Happy Little Scallop is RAW!"
	case "snoop_child":
		title = "Fo' Shizzle, My Soufflé"
	case "dr_seuss":
		title = "Oh, the Things You Will Find!"
	case "tolstoy_camus":
		title = "The Kingdom and the Absurd"
	case "bastion":
		title = "The Kid's Tale"
	case "diogenes_chesterton":
		title = "The Lamp and the Cross"
	case "thompson":
		title = "Loathing in the Dragon's Lair"
	case "fishburne":
		title = "A Glitch in the Code"
	case "blanchett":
		title = "A Whisper of Starlight"
	}

	pdf.CellFormat(0, 10, title, "", 1, "C", false, 0, "")
	pdf.Ln(10)

	pdf.SetFont("Times", "I", 16)
	subtitle := fmt.Sprintf("An AI-generated %s tale in the style of %s",
		cases.Title(language.English).String(sess.CurrentGenre),
		sess.CurrentAuthor,
	)
	pdf.CellFormat(0, 10, subtitle, "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetFont("Times", "", 12)
	difficulty := fmt.Sprintf("Difficulty: %s", cases.Title(language.English).String(sess.GameState.Rules.ConsequenceModel))
	pdf.CellFormat(0, 10, difficulty, "", 1, "C", false, 0, "")

	if sess.CurrentGenre == "historical-fiction" {
		pdf.Ln(20)
		pdf.SetFont("Times", "B", 14)
		pdf.CellFormat(0, 10, "Historical Context", "", 1, "C", false, 0, "")
		pdf.Ln(5)
		pdf.SetFont("Times", "", 12)
		contextText := fmt.Sprintf("Event: %s\n%s", sess.HistoricalEvent, sess.HistoricalDesc)
		pdf.MultiCell(0, 6, contextText, "", "C", false)
		pdf.Ln(2)
		pdf.SetFont("Times", "I", 10)
		pdf.SetTextColor(65, 105, 225) // RoyalBlue
		pdf.CellFormat(0, 10, sess.HistoricalURL, "", 1, "C", false, 0, sess.HistoricalURL)
		pdf.SetTextColor(0, 0, 0)
	}

	// Story Pages
	pdf.AddPage()
	pdf.SetFont("Times", "", 12)
	pdf.SetMargins(20, 20, 20)

	for _, page := range sess.StoryHistory {
		pdf.SetFont("Times", "I", 12)
		pdf.SetTextColor(64, 64, 64)
		pdf.MultiCell(0, 6, "> "+page.Prompt, "", "", false)
		pdf.Ln(6)

		pdf.SetFont("Times", "", 12)
		pdf.SetTextColor(0, 0, 0)
		writeHtmlToPdf(pdf, page.Response)
		pdf.Ln(12)
	}

	// Glossary Page
	if len(sess.GameState.ProperNouns) > 0 {
		pdf.AddPage()
		pdf.SetFont("Times", "B", 24)
		pdf.CellFormat(0, 10, "Glossary of Terms", "", 1, "C", false, 0, "")
		pdf.Ln(10)
		pdf.SetFont("Times", "", 12)
		for _, noun := range sess.GameState.ProperNouns {
			pdf.SetFont("Times", "B", 12)
			pdf.Write(6, noun.Noun+": ")
			pdf.SetFont("Times", "", 12)
			pdf.Write(6, noun.Description)
			pdf.Ln(8)
		}
	}

	var pdfBuffer bytes.Buffer
	err := pdf.Output(&pdfBuffer)
	if err != nil {
		log.Printf("Error generating PDF to buffer: %v", err)
		http.Error(w, "Failed to generate PDF.", http.StatusInternalServerError)
		return
	}

	go pingStatsService("upload-pdf", pdfBuffer.Bytes())

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=story.pdf")
	w.Write(pdfBuffer.Bytes())
}

// pingStatsService sends a POST request to the stats service.
// For sending PDFs, the pdfData should contain the raw PDF bytes.
func pingStatsService(endpoint string, pdfData []byte) {
	statsServiceURL := os.Getenv("STATS_SERVICE_URL") // Get URL from environment variable
	if statsServiceURL == "" {
		// You can set a default for local testing
		statsServiceURL = "http://localhost:8080"
	}

	url := fmt.Sprintf("%s/%s", statsServiceURL, endpoint)

	var req *http.Request
	var err error

	if pdfData != nil {
		req, err = http.NewRequest("POST", url, bytes.NewBuffer(pdfData))
		req.Header.Set("Content-Type", "application/pdf")
	} else {
		req, err = http.NewRequest("POST", url, nil)
	}

	if err != nil {
		log.Printf("Error creating request to stats service: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error pinging stats service at '%s': %v", endpoint, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Stats service returned non-OK status for '%s': %s", endpoint, resp.Status)
	} else {
		log.Printf("Successfully pinged stats service at '%s'", endpoint)
	}
}
