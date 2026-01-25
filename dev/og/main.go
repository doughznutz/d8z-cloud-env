package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// ==== Main ====
func main() {
	port := "11434"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	mux := http.NewServeMux()

	// Register provider-specific routes
	for _, provider := range ogProviders {
		// Route for OpenAI-compatible chat completions
		chatPath := fmt.Sprintf("%s/chat/completions", provider.APIBase)

		// Determine the correct handler based on provider name
		var handler http.HandlerFunc
		if provider.Name != "Gemini" { // Gemini is handled separately due to its URL structure
			switch provider.Name {
			case "Groq":
				handler = handleGroqChat
			case "OpenAI":
				handler = handleOpenAIChat
			default:
				log.Printf("Warning: No specific handler for %s, using default OpenAI handler", provider.Name)
				handler = handleOpenAIChat
			}
			if handler != nil {
				log.Printf("Registering OpenAI-compatible handler for %s at %s", provider.Name, chatPath)
				mux.HandleFunc(chatPath, handler)
			}
		} else {
			// Route for Gemini's native API structure, stripping the base path
			geminiPath := fmt.Sprintf("%s/", provider.APIBase)
			log.Printf("Registering Gemini native handler for %s at %s", provider.Name, geminiPath)
			mux.HandleFunc(geminiPath, stripPrefix(provider.APIBase, handleGeminiChat))
		}
	}
	// Legacy endpoint - for older Continue versions or direct calls
	mux.HandleFunc("/v1/chat/completions", handleOpenAIChat)

	// VSCode Continue extension YAML interface.
	mux.HandleFunc("/continue-yaml", handleContinueYaml)

	// These are ollama interfaces to say what models are available.
	mux.HandleFunc("/api/tags", handleTags)
	mux.HandleFunc("/api/show", handleShow)

	// Default route for unknown endpoints
	mux.HandleFunc("/", handleNotFound)
	
	log.Printf("Ollama Gatewai server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))

}

// stripPrefix is a helper function to remove a URL prefix before handling.

func stripPrefix(prefix string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
		h(w, r)
	}

}
