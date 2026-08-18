package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
	"log"
	"os"
	"story_ai/handlers"
	"story_ai/prompts"
	"story_ai/story"
	"time"
)

func main() {
	godotenv.Load("../.env")
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-3.1-flash-lite-preview")
	temp := float32(0.9)
	model.GenerationConfig = genai.GenerationConfig{
		Temperature:      &temp,
		ResponseMIMEType: "application/json",
	}

	systemInstruction := fmt.Sprintf(prompts.BasePrompt, "J.R.R. Tolkien") + prompts.FantasyPrompt
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemInstruction)},
	}

	initialRequest := handlers.AIRequest{
		GameState: &story.GameState{
			PlayerStatus:      story.PlayerStatus{Health: 100, Stamina: 100, Conditions: make([]string, 0)},
			Inventory:         make([]story.Item, 0),
			Environment:       story.Environment{Exits: make(map[string]string), WorldObjects: make([]story.WorldObject, 0)},
			NPCs:              make([]story.NPC, 0),
			Puzzles:           make([]story.Puzzle, 0),
			ProperNouns:       make([]story.ProperNoun, 0),
			Rules:             story.Rules{ConsequenceModel: "exploratory"},
			World:             story.World{WorldTension: 0},
			Climax:            false,
			WinConditions:     make([]string, 0),
			LossConditions:    make([]string, 0),
			SolvedPuzzleTypes: make([]string, 0),
		},
		UserAction: "Start the game.",
	}
	reqBytes, _ := json.Marshal(initialRequest)

	fmt.Println("Sending request...")
	start := time.Now()
	resp, err := model.GenerateContent(ctx, genai.Text(string(reqBytes)))
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("Received response in %v\n", time.Since(start))

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		fmt.Println(resp.Candidates[0].Content.Parts[0])
	}
}
