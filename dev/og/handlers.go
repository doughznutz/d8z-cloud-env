package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// handleTags reports what models are available.
// TODO: Only tell the models that are enabled by the keys available.
type ModelList struct {
	Models []ModelEntry `json:"models"`
}

type ModelEntry struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int    `json:"size"`
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	// log.Printf("Received Tags request: %v", r)
	w.Header().Set("Content-Type", "application/json")
	var modelEntries []ModelEntry
	for _, provider := range ogProviders {
		modelEntries = append(modelEntries,
			ModelEntry{
				// Name:    provider.Models[0],
				Name:       provider.Name,
				ModifiedAt: time.Now().Format(time.RFC3339),
				Size:       0,
			},
		)
	}
	models := ModelList{Models: modelEntries}
	json.NewEncoder(w).Encode(models)
}

// HandleShow gives a generice openai response for all models.
// Maybe we should use gemini for gemini models.
type ShowResponse struct {
	Model   string      `json:"model"`
	Details ModelDetail `json:"details"`
}
type ModelDetail struct {
	Parameters int    `json:"parameters"`
	Family     string `json:"family"`
	ModifiedAt string `json:"modified_at"`
}

func handleShow(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %v", r)
	model := r.URL.Query().Get("name")
	if model == "" {
		http.Error(w, "missing model name", http.StatusBadRequest)
		return
	}
	resp := ShowResponse{
		Model: model,
		Details: ModelDetail{
			Parameters: 0,
			Family:     "openai",
			ModifiedAt: time.Now().Format(time.RFC3339),
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received unhandled request: %v", r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	resp := map[string]string{
		"error":   fmt.Sprintf("endpoint not found: %v", r.URL.Path),
		"method":  r.Method,
		"path":    r.URL.Path,
		"message": "This Ollama-compatible proxy does not support the requested API.",
	}
	json.NewEncoder(w).Encode(resp)
}
