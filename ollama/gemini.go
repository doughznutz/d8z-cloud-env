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

// ==== Types ====

type GeminiRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings   []GeminiSafetySetting   `json:"safetySettings,omitempty"`
	Tools            []GeminiTool            `json:"tools,omitempty"`
}
type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}
type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

// GeminiSafetySetting defines the safety setting for a category.
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiTool describes a tool that the model may use to generate the next response.
type GeminiTool struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Function    GeminiFunctionCall `json:"function,omitempty"`
}

// GeminiFunctionCall describes a function that the model may call.
type GeminiFunctionCall struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` //JSON schema
}

// GeminiResponse is the JSON structure for responses returend from Gemini
type GeminiResponse struct {
	Candidates []struct {
		Content       GeminiContent        `json:"content"`
		FinishReason  string               `json:"finishReason"`
		SafetyRatings []GeminiSafetyRating `json:"safetyRatings"`
	} `json:"candidates"`
	PromptFeedback struct {
		SafetyRatings []GeminiSafetyRating `json:"safetyRatings"`
	} `json:"promptFeedback"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
}

// GeminiSafetyRating defines the safety rating for a piece of content.
type GeminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Severity    string `json:"severity"`
}

// GeminiUsageMetadata describes the usage metadata for a response.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ==== Conversion Routines ====

func OpenAICompletionsToGemini(req OpenAICompletionRequest) GeminiRequest {
	return GeminiRequest{
		Contents: []GeminiContent{
			{
				Role: "user",
				Parts: []GeminiPart{
					{Text: req.Prompt},
				},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}
}

func OpenAIChatToGemini(req OpenAIChatRequest) GeminiRequest {
	var geminiContents []GeminiContent
	for _, msg := range req.Messages {
		role := msg.Role
		if role != "user" && role != "model" {
			if role == "tool" {
				continue
			} // Discard tool role messages
			role = "user" // Default mapping (from system, etc)
		}
		geminiContents = append(geminiContents, GeminiContent{
			Role: role,
			Parts: []GeminiPart{{
				Text: msg.Content,
			}},
		})
	}
	//Basic safety settings.  These may need to be exposed to the user, but are hardcoded for now.
	safetySettings := []GeminiSafetySetting{
		{
			Category:  "HARM_CATEGORY_HARASSMENT",
			Threshold: "BLOCK_NONE", // "BLOCK_MEDIUM_AND_ABOVE",
		},
		{
			Category:  "HARM_CATEGORY_HATE_SPEECH",
			Threshold: "BLOCK_NONE", // "BLOCK_MEDIUM_AND_ABOVE",
		},
		{
			Category:  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
			Threshold: "BLOCK_NONE", // "BLOCK_MEDIUM_AND_ABOVE",
		},
		{
			Category:  "HARM_CATEGORY_DANGEROUS_CONTENT",
			Threshold: "BLOCK_NONE", // "BLOCK_MEDIUM_AND_ABOVE",
		},
	}
	return GeminiRequest{
		Contents:       geminiContents,
		SafetySettings: safetySettings,
	}
}

// ==== Gemini Functions ====

func makeRequestToGemini(w http.ResponseWriter, req GeminiRequest) (*GeminiResponse, error) {
	var geminiResp GeminiResponse
	reqBody, err := json.Marshal(req)
	if err != nil {
		log.Printf("Gemini JSON marshalling error: %v", err)
	}
	// Make request to Gemini
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=" + geminiAPIKey
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Printf("Gemini error: %v", err)
		http.Error(w, "Gemini request failed", http.StatusBadGateway)
		return &geminiResp, err
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Gemini response error: %s", body)
		http.Error(w, "Gemini API error - "+string(body), resp.StatusCode)
		return &geminiResp, fmt.Errorf("Gemini API returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		http.Error(w, "Failed to parse Gemini response", http.StatusInternalServerError)
		return &geminiResp, err
	}
	log.Printf("Gemini Response: %v", geminiResp)
	return &geminiResp, nil
}

func sendGeminiToOpenAICompletion(w http.ResponseWriter, geminiResp *GeminiResponse) {
	var outputText string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		outputText = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	now := time.Now().Unix()
	resp := OpenAICompletionResponse{
		ID:      "cmpl-gemini-fake-id",
		Object:  "text_completion",
		Created: now,
		Model:   "gemini-2.0-flash", // model,
		Choices: []OpenAICompletionChoice{
			{
				Text:         outputText,
				Index:        0,
				Logprobs:     nil,
				FinishReason: "stop",
			},
		},
		Usage: OpenAICompletionUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}
	json.NewEncoder(w).Encode(resp)
}
func streamGeminiToOpenAIChat(w http.ResponseWriter, resp *GeminiResponse) {
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
	if len(resp.Candidates) == 0 {
		http.Error(w, "No response from Gemini", http.StatusInternalServerError)
		return
	}

	parts := resp.Candidates[0].Content.Parts
	for _, part := range parts {
		streamChunk := map[string]interface{}{
			"id":      "chatcmpl-gemini",
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   "gemini-2.0-flash", // chatReq.Model,
			"choices": []map[string]interface{}{{
				"delta": map[string]string{
					"content": part.Text,
				},
				"index":         0,
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
