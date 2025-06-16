package main

import (
	"log"
	"net/http"
	"os"
)

var openaiAPIKey = os.Getenv("OPENAI_API_KEY")
var geminiAPIKey = os.Getenv("GEMINI_API_KEY")
var groqAPIKey = os.Getenv("GROQ_API_KEY")

// ==== Main ====
func main() {
	port := "11434"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleOpenAIChat)
	//mux.HandleFunc("/v1/chat/completions", handleGroqChat)
	//mux.HandleFunc("/v1/completions", handleOpenAICompletions)
	//mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/tags", handleTags)
	mux.HandleFunc("/api/show", handleShow)
	mux.HandleFunc("/v1/api/show", handleShow)

	//mux.HandleFunc("/api/delete", handleMockSuccess)
	//mux.HandleFunc("/api/pull", handleMockSuccess)

	// Default route for unknown endpoints
	mux.HandleFunc("/", handleNotFound)

	log.Printf("Ollama proxy server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
