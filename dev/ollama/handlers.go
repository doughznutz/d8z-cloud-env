package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	models := ModelList{
		Models: []ModelEntry{
			{Name: "gemini-2.0-flash", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
			{Name: "gpt-4.1", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
			{Name: "llama-3.3-70b-versatile", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
		},
	}
	json.NewEncoder(w).Encode(models)
}

// HandleGenerate is legacy and currently unused.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
	Stream      bool    `json:"stream"`
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received Generate request: %v", r)
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

func handleMockSuccess(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request: %v", r)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "success"}`))
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

// OpenAI v1/completions-style legacy
type OpenAICompletionRequest struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type OpenAICompletionResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAICompletionChoice `json:"choices"`
	Usage   OpenAICompletionUsage    `json:"usage"`
}

type OpenAICompletionChoice struct {
	Text         string      `json:"text"`
	Index        int         `json:"index"`
	Logprobs     interface{} `json:"logprobs"` // Always null in completions
	FinishReason string      `json:"finish_reason"`
}

type OpenAICompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func handleOpenAICompletions(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received V1/Completion request: %v", r.Body)

	var completionReq OpenAICompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&completionReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	geminiReq := OpenAICompletionsToGemini(completionReq)
	geminiResp, err := makeRequestToGemini(w, geminiReq)
	if err != nil {
		return
	}
	sendGeminiToOpenAICompletion(w, geminiResp)
}
