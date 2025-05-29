package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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

type OpenAIChatStream struct {
	Data []OpenAIChatResponse       `json:"data"`
}
type OpenAIChatResponse struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Created int64                   `json:"created"`
	Choices []OpenAIChatChoice      `json:"choices"`
	Usage   OpenAIChatCompletionUsage `json:"usage"`
}
type OpenAIChatChoice struct {
	Index        int              `json:"index"`
	FinishReason string           `json:"finish_reason"`
	Delta        OpenAIChatMessage `json:"delta"`	
	Message      OpenAIChatMessage `json:"message"`

}

type OpenAIChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Index int `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
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

	// Override, I think this is a voideditor bug for Ollama
	chatReq.Stream = true

	// Start the database log entry
	dbEntry := &OpenAILog {
		Timestamp: time.Now(),
		UserID: "doughznutz",  // Replace with reverse DNS of container requesting.
		Model: chatReq.Model,
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
		if err != nil {return}
    	streamGeminiToOpenAIChat(w, geminiResp)

		// We need to convert this to openAIchat response.
		dbEntry.Response, err = json.Marshal(geminiResp)
		if err != nil {
			log.Println("Error marshalling geminiReq to JSON:", err)
		}
	
		// This function shouldnt bother returning...just log the error inside it.
		if err := insert_into_openai_logs(dbEntry); err != nil {
			log.Printf("Error inserting into database: %v", err)
		}
		return
	}

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

	//scanner := bufio.NewScanner(resp.Body)
	openAIChatResponse := OpenAIChatResponse{}
	scanner := bufio.NewScanner(bytes.NewBuffer(responseBody))
	for scanner.Scan() {
		line := scanner.Text()
		if bytes.HasPrefix([]byte(line), []byte("data: ")) {
			data := bytes.TrimPrefix([]byte(line), []byte("data: "))
			var response OpenAIChatResponse
			if err := json.Unmarshal(data, &response); err != nil {
				log.Printf("Error unmarshaling data: %v", err)
				continue
			}
			if openAIChatResponse.ID == "" {
				openAIChatResponse = response
			} else {
				openAIChatResponse.Choices[0].Delta.Content += response.Choices[0].Delta.Content
			}
		} else if line == "" {
			// Heartbeat or empty line
			continue
		} else {
			// Other lines (e.g., comments)
			log.Printf("Ignoring line: %s", line)
		}
	}

	fmt.Printf("Received: %+v\n", openAIChatResponse)
	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading stream: %v", err)
	}	
	dbEntry.Response, err = json.Marshal(openAIChatResponse)
	if err != nil {
		log.Println("Error marshalling OpenAIChatResponse to JSON:", err)
	}
	

	// This function shouldnt bother returning...just log the error inside it.
	if err := insert_into_openai_logs(dbEntry); err != nil {
		log.Printf("Error inserting into database: %v", err)
	}
}

