package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

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

func main() {
	url := "http://localhost:11434/api/chat"

	reqBody := ChatRequest{
		Model: "gpt-3.5-turbo",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello, what is the capital of France?"},
		},
		Temperature: 0.7,
		Stream:      false, // Set to true to test streaming
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		panic("Failed to encode request: " + err.Error())
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		panic("Request failed: " + err.Error())
	}
	defer resp.Body.Close()

	fmt.Println("Response:")
	if reqBody.Stream {
		// Stream response chunk-by-chunk
		buf := make([]byte, 1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				fmt.Print(string(buf[:n]))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				panic("Error reading stream: " + err.Error())
			}
		}
	} else {
		// Buffer full response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			panic("Failed to read response: " + err.Error())
		}
		fmt.Println(string(body))
	}
}