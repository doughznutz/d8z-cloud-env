package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"time"
	"net/http"
)

const openaiURL = "https://api.openai.com/v1/chat/completions"

// OpenAI v1/completions-style legacy
type OpenAICompletionRequest struct {
	Prompt     string  `json:"prompt"`
	MaxTokens  int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type OpenAICompletionResponse struct {
	ID      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []OpenAICompletionChoice   `json:"choices"`
	Usage   OpenAICompletionUsage      `json:"usage"`
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


type OpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []OpenAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}


// ==== Handlers ====
func handleOpenAICompletions(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received V1/Completion request: %v", r.Body)

	var completionReq OpenAICompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&completionReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	geminiReq := OpenAICompletionsToGemini(completionReq)
	geminiResp, err := makeRequestToGemini(w, geminiReq)
	if err != nil {return}
    sendGeminiToOpenAICompletion(w, geminiResp)
}

func handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received V1/Chat/Completion request: %v", r.Body)

	var chatReq OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Branch off here to Gemini baed on chatReq.Model
	if chatReq.Model == "gemini-2.0-flash" {
		geminiReq := OpenAIChatToGemini(chatReq)
		geminiResp, err := makeRequestToGemini(w, geminiReq)
		if err != nil {return}
    	streamGeminiToOpenAIChat(w, geminiResp)
		return
	}

	chatReq.Stream = true
	chatReqBody, err := json.Marshal(chatReq)
	if err != nil {
		http.Error(w, "failed to marshal chat request body", http.StatusInternalServerError)
		return
	}

	openaiReq, err := http.NewRequest("POST", openaiURL, bytes.NewBuffer(chatReqBody))
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
	// Read the entire response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		http.Error(w, "Error reading response from OpenAI", http.StatusInternalServerError)
		return
	}

	// Restore the original response body for streaming
	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying stream: %v", err)
	}

	// Ensure the connection is closed
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	dbEntry := &OpenAILog {
		Timestamp: time.Now(),
		UserID: "doughznutz",  // Replace with reverse DNS of container requesting.
		Model: chatReq.Model,
		Request: chatReqBody,
		Response: responseBody,
	}

	// This function shouldnt bother returning...just log the error inside it.
	if err := insert_into_openai_logs(dbEntry); err != nil {
		log.Printf("Error inserting into database: %v", err)
	}
}

