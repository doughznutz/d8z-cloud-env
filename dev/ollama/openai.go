package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

const openaiURL = "https://api.openai.com/v1/chat/completions"
const groqURL = "https://api.groq.com/openai/v1/chat/completions"

// OpenAIChatRequest is the JSON structure for requests to OpenAI
type OpenAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []OpenAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}
type OpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIChatResponse is the JSON structure for what is returned from OpenAI
type OpenAIChatResponse struct {
	ID                string                    `json:"id"`
	Object            string                    `json:"object"`
	Created           int64                     `json:"created"`
	Model             string                    `json:"model"`
	SystemFingerprint string                    `json:"system_fingerprint,omitempty"`
	Choices           []OpenAIChatChoice        `json:"choices"`
	Usage             OpenAIChatCompletionUsage `json:"usage"`
}
type OpenAIChatChoice struct {
	Index        int               `json:"index"`
	FinishReason string            `json:"finish_reason"`
	Delta        OpenAIChatMessage `json:"delta"`
	Message      OpenAIChatMessage `json:"message"`
	Logprobs     interface{}       `json:"logprobs"`
}

type OpenAIChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ==== Handlers ====

func handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received V1/Chat/Completion request: %v", r.Body)

	var chatReq OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Override, I think this is a voideditor bug for Ollama
	// chatReq.Stream = true

	// Start the database log entry
	dbEntry := &OpenAILog{
		Timestamp: time.Now(),
		UserID:    "doughznutz", // Replace with reverse DNS of container requesting.
		Model:     chatReq.Model,
	}
	var err error
	dbEntry.Request, err = json.Marshal(chatReq)
	if err != nil {
		log.Println("Error marshalling chatReq to JSON:", err)
	}

	// Branch off here to Gemini baed on chatReq.Model
	if chatReq.Model == "gemini-2.0-flash" {
		geminiReq := OpenAIChatToGemini(chatReq)
		geminiResp, err := makeRequestToGemini(w, geminiReq)
		if err != nil {
			return
		}
		streamGeminiToOpenAIChat(w, geminiResp)

		// We need to convert this to openAIchat response.
		dbEntry.Response, err = json.Marshal(geminiResp)
		if err != nil {
			log.Println("Error marshalling geminiReq to JSON:", err)
		}

		// This function shouldnt bother returning...just log the error inside it.
		if err := insert_into_ollama_logs(dbEntry); err != nil {
			log.Printf("Error inserting into database: %v", err)
		}
		return
	}

	chatReqBody, err := json.Marshal(chatReq)
	if err != nil {
		http.Error(w, "failed to marshal chat request body", http.StatusInternalServerError)
		return
	}

	reqURL, reqAPIKey := openaiURL, openaiAPIKey
	if chatReq.Model == "llama-3.3-70b-versatile" {
		reqURL, reqAPIKey = groqURL, groqAPIKey
	}

	openaiReq, err := http.NewRequest("POST", reqURL, bytes.NewBuffer(chatReqBody))
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	openaiReq.Header.Set("Authorization", "Bearer "+reqAPIKey)
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

	var buf bytes.Buffer
	mw := io.MultiWriter(w, &buf)

	// Copy the response stream to the client and buffer
	_, err = io.Copy(mw, resp.Body)
	if err != nil {
		log.Printf("Error copying stream: %v", err)
		return
	}

	// Ensure the connection is closed and buffer flushed
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Log the entire response
	//dbEntry.Response = buf.Bytes()
	dbEntry.Response, err = stream_into_ollama_logs(buf)
	if err != nil {
		log.Println("Error marshalling OpenAIChatResponse to JSON:", err)
	}

	// This function shouldnt bother returning...just log the error inside it.
	if err := insert_into_ollama_logs(dbEntry); err != nil {
		log.Printf("Error inserting into database: %v", err)
	}
}
