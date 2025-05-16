package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const openAIURL = "http://api.openai.com/v1/chat/completions"

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	Stream     bool          `json:"stream,omitempty"`
}

type OpenAIRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OllamaRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	openAIReq := OpenAIRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		Stream:      req.Stream,
	}

	payload, err := json.Marshal(openAIReq)
	if err != nil {
		http.Error(w, "Failed to encode request", http.StatusInternalServerError)
		return
	}

	openAIKey := os.Getenv("OPENAI_API_KEY")
	if openAIKey == "" {
		http.Error(w, "OPENAI_API_KEY not set", http.StatusInternalServerError)
		return
	}

	client := &http.Client{
		Timeout: time.Minute,
	}

	openAIRequest, _ := http.NewRequest("POST", openAIURL, bytes.NewReader(payload))
	openAIRequest.Header.Set("Authorization", "Bearer "+openAIKey)
	openAIRequest.Header.Set("Content-Type", "application/json")

	openAIResp, err := client.Do(openAIRequest)
	if err != nil {
		http.Error(w, "Failed to call OpenAI API: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer openAIResp.Body.Close()

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Stream raw OpenAI chunks to client
		buf := make([]byte, 1024)
		for {
			n, err := openAIResp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				w.(http.Flusher).Flush()
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Println("Stream error:", err)
				break
			}
		}
	} else {
		// Buffer full response and extract content
    type OpenAIResponse struct {
      Choices []struct {
        Text string `json:"text"`
      } `json:"choices"`
    }
    var responseBody struct {
			Choices []struct {
				Text string `json:"text"`
			} `json:"choices"`
			//Created int64  `json:"created"`
			//Model   string `json:"model"`
		}

		err = json.NewDecoder(openAIResp.Body).Decode(&responseBody)
		if err != nil {
      log.Printf("OpenAI response %v", openAIResp.Body)
			http.Error(w, "Failed to parse OpenAI response", http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"message": map[string]string{
				"role":    "assistant",
				//"content": responseBody.Choices[0].Message.Content,
			},
			//"created_at": responseBody.Created,
			//"model":      responseBody.Model,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func main() {
	http.HandleFunc("/api/chat", handler)
	log.Println("Ollama proxy listening on :11434")
	log.Fatal(http.ListenAndServe(":11434", nil))
}