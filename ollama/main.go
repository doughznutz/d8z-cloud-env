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

var geminiAPIKey = os.Getenv("GEMINI_API_KEY")

type OpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatRequest struct {
	Model    string               `json:"model"`
	Messages []OpenAIChatMessage `json:"messages"`
	Stream   bool                 `json:"stream"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content GeminiContent `json:"content"`
	} `json:"candidates"`
}


// ==== Handlers ====
func handleChat(w http.ResponseWriter, r *http.Request) {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var bodyMap map[string]interface{}
	err = json.Unmarshal(reqBody, &bodyMap)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	model, ok := bodyMap["model"].(string)
	if !ok {
		http.Error(w, "model not found", http.StatusBadRequest)
		return
	}

	switch model {
	case "gpt4.1":
		handleOpenAIChat(w, r)
	default:
		handleGeminiChat(w, r)
	}
}

func handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received OpenAI request: %v", r.Body)

	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Force stream to true
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(reqBody, &bodyMap); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	bodyMap["stream"] = true
	updatedReqBody, err := json.Marshal(bodyMap)
	if err != nil {
		http.Error(w, "failed to marshal updated request body", http.StatusInternalServerError)
		return
	}

	openaiReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(updatedReqBody))
	if err != nil {
		http.Error(w, "failed to create OpenAI request", http.StatusInternalServerError)
		return
	}

	openaiReq.Header.Set("Authorization", "Bearer "+openaiAPIKey)
	openaiReq.Header.Set("Content-Type", "application/json")
	openaiReq.Header.Set("Accept", "text/event-stream") // Important for SSE

	client := &http.Client{}
	resp, err := client.Do(openaiReq)
	if err != nil {
		http.Error(w, "error contacting OpenAI", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("OpenAI API returned an error: %v", resp.Status)
		http.Error(w, "OpenAI API error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Copy the response stream to the client
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying stream: %v", err)
	}

	// Ensure the connection is closed
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func handleGeminiChat(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received Gemini request: %v", r.Body)

	var openAIReq OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&openAIReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Translate to Gemini request format
	var geminiContents []GeminiContent
	for _, msg := range openAIReq.Messages {
		geminiContents = append(geminiContents, GeminiContent{
			Role: msg.Role,
			Parts: []GeminiPart{{
				Text: msg.Content,
			}},
		})
	}
	geminiReq := GeminiRequest{Contents: geminiContents}
	reqBody, _ := json.Marshal(geminiReq)

	// Make request to Gemini
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + geminiAPIKey
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		http.Error(w, "Gemini request failed", http.StatusBadGateway)
		log.Printf("Gemini error: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		http.Error(w, "Failed to parse Gemini response", http.StatusInternalServerError)
		return
	}
	log.Printf("Gemini Response: %v", geminiResp)
	// Prepare for OpenAI-style streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Convert Gemini content to OpenAI streaming format
	if len(geminiResp.Candidates) == 0 {
		http.Error(w, "No response from Gemini", http.StatusInternalServerError)
		return
	}

	parts := geminiResp.Candidates[0].Content.Parts
	for _, part := range parts {
		streamChunk := map[string]interface{}{
			"id":      "chatcmpl-gemini",
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   openAIReq.Model,
			"choices": []map[string]interface{}{{
				"delta": map[string]string{
					"content": part.Text,
				},
				"index":        0,
				"finish_reason": nil,
			}},
		}

		chunkJSON, _ := json.Marshal(streamChunk)
		fmt.Fprintf(w, "data: %s\n\n", chunkJSON)
		flusher.Flush()
	}

	// Final DONE message
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}



func handleGenerate(w http.ResponseWriter, r *http.Request) {
  log.Printf("Received request: %v", r)
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
  // log.Printf("Received request: %v", r)
	w.Header().Set("Content-Type", "application/json")
	models := ModelList{
		Models: []ModelEntry{
			{Name: "gemini-2.0-flash", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
			{Name: "gpt-4.1", ModifiedAt: time.Now().Format(time.RFC3339), Size: 0},
		},
	}
	json.NewEncoder(w).Encode(models)
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
  log.Printf("Received request: %v", r)
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
	mux.HandleFunc("/v1/chat/completions", handleGeminiChat)
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