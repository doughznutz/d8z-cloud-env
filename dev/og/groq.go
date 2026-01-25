
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

func handleGroqChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Decode the incoming request body
	var chatReq ChatCompletionRequest
	
	// Read the body first to allow for logging
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the body for subsequent reads

	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		http.Error(w, "invalid request json", http.StatusBadRequest)
		return
	}

	// 2. Get provider details
	provider := getProviderByModel(chatReq.Model)
	if provider == nil {
		http.Error(w, "unsupported model", http.StatusBadRequest)
		return
	}
	
	// Replace provider name with actual model name for the upstream request
	chatReq.Model = provider.Models[0]


	// 3. Create the outgoing request to the provider
	// Re-encode the modified request
	proxyBodyBytes, err := json.Marshal(chatReq)
	if err != nil {
		http.Error(w, "failed to re-encode request body", http.StatusInternalServerError)
		return
	}

	proxyReq, err := http.NewRequestWithContext(
		ctx,
		"POST",
		provider.URL+"/chat/completions",
		bytes.NewReader(proxyBodyBytes),
	)
	if err != nil {
		http.Error(w, "request creation failed", http.StatusInternalServerError)
		return
	}

	// 4. Set headers
	proxyReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 4.5 Log the outgoing request for debugging
	log.Printf("--> [GROQ] Forwarding request to: %s", proxyReq.URL)
	log.Printf("--> [GROQ] Method: %s", proxyReq.Method)
	log.Printf("--> [GROQ] Headers:")
	for key, values := range proxyReq.Header {
		for _, value := range values {
			// Do not log the full API key
			if key == "Authorization" {
				log.Printf("    %s: Bearer %s...", key, provider.APIKey[:4])
			} else {
				log.Printf("    %s: %s", key, value)
			}
		}
	}

	// 5. Execute the request
	client := &http.Client{}
	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "API request to provider failed", http.StatusBadGateway)
		return
	}
	defer proxyResp.Body.Close()

	// 6. Stream the response back to the original client
	// Copy headers from the provider's response to our response
    for key, values := range proxyResp.Header {
        for _, value := range values {
            w.Header().Add(key, value)
        }
    }
	w.WriteHeader(proxyResp.StatusCode)

	// TeeReader allows us to read the response body while also capturing it for logging
	responseBodyBuffer := new(bytes.Buffer)
	tee := io.TeeReader(proxyResp.Body, responseBodyBuffer)

	_, err = io.Copy(w, tee)
	if err != nil {
		log.Printf("Error streaming response to client: %v", err)
	}

	// 7. Log the request and response
	reqLog, _ := json.Marshal(json.RawMessage(bodyBytes)) // Log the original request
	respLog, _ := json.Marshal(json.RawMessage(responseBodyBuffer.Bytes()))

	dbEntry := &OpenAILog{
		Timestamp: time.Now(),
		UserID:    "doughznutz", // Or extract from request if available
		Model:     provider.Name, // Log the provider name
		Request:   reqLog,
		Response:  respLog,
	}

	if err := log_openai_event(dbEntry); err != nil {
		log.Printf("Error logging event: %v", err)
	}
}

