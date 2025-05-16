package main

import (
	"bytes"
	"encoding/json"
  "fmt"
  "io"
	"log"
	"net/http"
	"os"
	"time"
)

const openaiURL = "https://api.openai.com/v1/chat/completions"

var openaiAPIKey = os.Getenv("OPENAI_API_KEY")

// ==== Types ====

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type CompletionRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
	Stream      bool    `json:"stream"`
}

type ModelList struct {
	Models []ModelEntry `json:"models"`
}

type ModelEntry struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int    `json:"size"`
}

type ShowResponse struct {
	Model  string       `json:"model"`
	Details ModelDetail `json:"details"`
}

type ModelDetail struct {
	Parameters int    `json:"parameters"`
	Family     string `json:"family"`
	ModifiedAt string `json:"modified_at"`
}

// ==== Handlers ====

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	openaiReq := map[string]interface{}{
		"model":       req.Model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"stream":      req.Stream,
	}

	body, _ := json.Marshal(openaiReq)
	httpReq, _ := http.NewRequest("POST", openaiURL, bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "openai request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	chatFormat := map[string]interface{}{
		"model": req.Model,
		"messages": []ChatMessage{
			{Role: "user", Content: req.Prompt},
		},
		"temperature": req.Temperature,
		"stream":      req.Stream,
	}

	body, _ := json.Marshal(chatFormat)
	httpReq, _ := http.NewRequest("POST", openaiURL, bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, "openai request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
  log.Printf("OpenAI Response %v", resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	models := ModelList{
		Models: []ModelEntry{
			{Name: "gpt-3.5-turbo", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
			{Name: "gpt-4", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
		},
	}
	json.NewEncoder(w).Encode(models)
}

func handleShow(w http.ResponseWriter, r *http.Request) {
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

func handleMockSuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "success"}`))
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
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

// ==== Main ====

func main() {
	port := "11434"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChat)
	mux.HandleFunc("/api/chat", handleChat)
	mux.HandleFunc("/api/generate", handleGenerate)
	mux.HandleFunc("/api/tags", handleTags)
	mux.HandleFunc("/api/show", handleShow)
	mux.HandleFunc("/api/delete", handleMockSuccess)
	mux.HandleFunc("/api/pull", handleMockSuccess)

  // Default route for unknown endpoints
  mux.HandleFunc("/", handleNotFound)

	log.Printf("Ollama proxy server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}