package main

import (
	"context"
	"fmt"
	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
	"log"
	"os"
)

func main() {
	godotenv.Load("../.env")
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := "models/gemini-1.5-flash-002"

	cc := &genai.CachedContent{
		Model: model,
		SystemInstruction: &genai.Content{
			Parts: []genai.Part{genai.Text("You are a helpful assistant. This is a very short system prompt.")},
		},
	}

	_, err = client.CreateCachedContent(ctx, cc)
	if err != nil {
		fmt.Printf("Cache creation error: %v\n", err)
	} else {
		fmt.Println("Cache created successfully!")
	}
}
