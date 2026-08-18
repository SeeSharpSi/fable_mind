package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"story_ai/handlers"
	"story_ai/llm"
	"story_ai/metrics"
	"story_ai/session"
	"story_ai/templates"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file, but don't crash if it's not present
	godotenv.Load()

	// Initialize metrics collector
	metricsURL := os.Getenv("METRICS_DATABASE_URL")
	if metricsURL == "" {
		metricsURL = "http://localhost:8081" // Default metrics database URL
	}
	metrics.InitDefaultCollector(metricsURL)

	ctx := context.Background()
	aiClient, err := llm.NewFromEnv(ctx)
	if err != nil {
		log.Fatalf("initialize LLM: %v", err)
	}
	defer aiClient.Close()
	log.Printf("Using %s LLM provider", aiClient.Provider())

	sessionManager := session.NewManager()

	h := &handlers.Handler{
		LLM:     aiClient,
		Manager: sessionManager,
	}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		templates.Index("Interactive Story").Render(context.Background(), w)
	})

	// Health check endpoints
	mux.HandleFunc("/health", handlers.HealthCheckHandler)
	mux.HandleFunc("/ready", handlers.ReadinessHandler)

	// Metrics endpoint
	mux.HandleFunc("/metrics", metrics.GetMetricsEndpoint())

	mux.HandleFunc("/start", h.StartStory)
	mux.HandleFunc("/generate", h.Generate)
	mux.HandleFunc("/download", h.DownloadStory)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9779"
	}

	log.Println("Listening on http://0.0.0.0:" + port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
