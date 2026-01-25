package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// --- Gemini API Translation Structs ---

// GeminiTool represents a collection of function declarations for the Gemini API.
type GeminiTool struct {
	FunctionDeclarations []*GeminiFunctionDeclaration `json:"functionDeclarations"`
}

// GeminiFunctionDeclaration defines a single function the model can call.
type GeminiFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// GeminiFunctionCall is what the model returns when it wants to call a function.
type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// GeminiRequest represents the request body for the Gemini API's generateContent method.
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
	Tools    []*GeminiTool   `json:"tools,omitempty"`
}

// GeminiContent represents a single message or turn in the conversation.
type GeminiContent struct {
	Role  string       `json:"role"` // "user" or "model"
	Parts []GeminiPart `json:"parts"`
}

// OpenAIMessage is a simplified struct for a message in an OpenAI-compatible request.
type OpenAIMessage struct {
	Role       string                 `json:"role"`
	Content    string                 `json:"content,omitempty"` // Content can be omitted for tool calls
	ToolCalls  []OpenAIToolCall       `json:"tool_calls,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
}

// GeminiFunctionResponse is used to provide the result of a function call.
type GeminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

// GeminiPart represents a piece of content, which can be text, a function call, or a function response.
type GeminiPart struct {
	Text             *string               `json:"text,omitempty"`
	FunctionCall     *GeminiFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// ChatCompletionRequestForGemini is a simplified struct for an OpenAI-compatible request body,
// used specifically for translating requests to Gemini, and includes fields for tool calls.
type ChatCompletionRequestForGemini struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Tools    []OpenAITool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
}

// translateOpenAItoGemini converts an OpenAI-compatible chat request to a native Gemini API request.
func translateOpenAItoGemini(openaiReq ChatCompletionRequestForGemini) GeminiRequest {
	geminiReq := GeminiRequest{
		Contents: make([]GeminiContent, 0),
	}
	var systemPrompt string

	for _, msg := range openaiReq.Messages {
		if msg.Role == "system" {
			systemPrompt += msg.Content + "\n"
			continue // System messages are prepended to the first user message
		}

		// Initialize content and parts for the current message
		var geminiContent GeminiContent
		var geminiParts []GeminiPart

		switch msg.Role {
		case "user":
			geminiContent.Role = "user"
			content := msg.Content
			// Prepend system prompt to the first user message only
			if systemPrompt != "" && len(geminiReq.Contents) == 0 {
				content = systemPrompt + content
			}
			geminiParts = append(geminiParts, GeminiPart{Text: &content})

		case "assistant":
			geminiContent.Role = "model"
			if len(msg.ToolCalls) > 0 {
				// This is an assistant message requesting tool calls
				for _, tc := range msg.ToolCalls {
					argsMap := make(map[string]interface{})
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err != nil {
						log.Printf("Error unmarshaling tool call arguments: %v", err)
						continue
					}
					geminiParts = append(geminiParts, GeminiPart{
							FunctionCall: &GeminiFunctionCall{
								Name: tc.Function.Name,
								Args: argsMap,
							},
						})
				}
			} else if msg.Content != "" {
				// Regular assistant text response
				content := msg.Content
				geminiParts = append(geminiParts, GeminiPart{Text: &content})
			}

		case "tool":
			// This is a tool response message
			geminiContent.Role = "function"
			var toolResponse interface{}
			// Attempt to unmarshal the tool content as JSON, fallback to string if not.
			if err := json.Unmarshal([]byte(msg.Content), &toolResponse); err != nil {
				log.Printf("Warning: tool content is not valid JSON, sending as string: %v, content: %s", err, msg.Content)
				toolResponse = msg.Content
			}

			// Gemini expects the actual function name here, not the tool_call_id
			// We need to infer the function name from the assistant's previous tool_calls or require it in OpenAIMessage
			// For simplicity, let's assume the function name is directly in msg.ToolCallID for now, or use a placeholder.
			functionName := msg.ToolCallID // This is a simplification; a proper mapping might be needed.
			if functionName == "" {
				functionName = "unknown_tool_function"
			}

			geminiParts = append(geminiParts, GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					Name: functionName,
					Response: map[string]interface{}{"result": toolResponse},
				},
			})
		}
		// Only add content if there are parts to avoid empty turns
		if len(geminiParts) > 0 {
			geminiContent.Parts = geminiParts
			geminiReq.Contents = append(geminiReq.Contents, geminiContent)
		}
	}

	// Translate tools (function declarations) if they exist
	if len(openaiReq.Tools) > 0 {
		var declarations []*GeminiFunctionDeclaration
		for _, tool := range openaiReq.Tools {
			if tool.Type == "function" && tool.Function != nil {
				declarations = append(declarations, &GeminiFunctionDeclaration{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				})
			}
		}
		if len(declarations) > 0 {
			geminiReq.Tools = []*GeminiTool{{FunctionDeclarations: declarations}}
		}
	}

	return geminiReq
}

// convertArgsToString is a helper to safely marshal the arguments map to a JSON string.
func convertArgsToString(args map[string]interface{}) string {
	argBytes, err := json.Marshal(args)
	if err != nil {
		log.Printf("Error marshaling function call arguments: %v", err)
		return "{}"
	}
	return string(argBytes)
}

// translateGeminiToOpenAIStream reads from the Gemini response stream, translates each chunk
// to the OpenAI format, and writes it to the client's ResponseWriter.
func translateGeminiToOpenAIStream(w http.ResponseWriter, geminiBody io.Reader, modelName string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	bodyBytes, err := io.ReadAll(geminiBody)
	if err != nil {
		log.Printf("Error reading from gemini stream: %v", err)
		return err
	}

	var geminiChunks []struct {
		Candidates []struct {
			Content struct {
				Parts []GeminiPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(bodyBytes, &geminiChunks); err != nil {
		log.Printf("Error unmarshaling gemini chunks: %v, data: %s", err, string(bodyBytes))
		// Attempt to unmarshal as a single object if array fails (for non-streaming responses that might get here)
		var singleChunk struct {
			Candidates []struct {
				Content struct {
					Parts []GeminiPart `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err_single := json.Unmarshal(bodyBytes, &singleChunk); err_single != nil {
			log.Printf("Failed to unmarshal as single chunk as well: %v", err_single)
			return err
		}
		geminiChunks = append(geminiChunks, singleChunk)
	}

	for _, geminiChunk := range geminiChunks {
		if len(geminiChunk.Candidates) > 0 && len(geminiChunk.Candidates[0].Content.Parts) > 0 {
			part := geminiChunk.Candidates[0].Content.Parts[0]

			openAIChunk := ChatCompletionStreamChunk{
				ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   modelName,
				Choices: []struct {
					Delta struct {
						Content   string           `json:"content,omitempty"`
						ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason,omitempty"`
				}{
					{}, // Initialize the choice
				},
			}

			if part.FunctionCall != nil {
				// It's a tool call
				toolCall := OpenAIToolCall{
					Index: 0,
					ID:    fmt.Sprintf("call_%d", time.Now().UnixNano()),
					Type:  "function",
					Function: struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{
						Name:      part.FunctionCall.Name,
						Arguments: convertArgsToString(part.FunctionCall.Args),
					},
				}
				openAIChunk.Choices[0].Delta.ToolCalls = []OpenAIToolCall{toolCall}

			} else if part.Text != nil {
				// It's a regular text response
				openAIChunk.Choices[0].Delta.Content = *part.Text
			}

			chunkBytes, err := json.Marshal(openAIChunk)
			if err != nil {
				log.Printf("Error marshaling openAI chunk: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	return nil
}


func handleGeminiChat(w http.ResponseWriter, r *http.Request) {



	// 1. Read the incoming request body



	bodyBytes, err := io.ReadAll(r.Body)



	if err != nil {



		http.Error(w, "failed to read request body", http.StatusInternalServerError)



		return



	}



	// Log the raw request body for debugging



	log.Printf("--> [GEMINI] Raw Request Body: %s", string(bodyBytes))







	// Restore the body so it can be read again



	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var modelName string

	var bodyToForward []byte

	isStreaming := false // Default to non-streaming



	// 2. Check if the request is OpenAI-compatible or native Gemini and prepare the body

	if strings.Contains(r.URL.Path, "/chat/completions") {

		log.Println("--> [GEMINI] Detected OpenAI-compatible chat completions request")
		

		var chatReq ChatCompletionRequest

		if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {

			http.Error(w, "failed to unmarshal OpenAI request body: "+err.Error(), http.StatusBadRequest)

			return

		}

		modelName = chatReq.Model

		isStreaming = chatReq.Stream



		// Manually construct the request for Gemini translation from the parsed body

		var messages []OpenAIMessage

		for _, msgMap := range chatReq.Messages {

			msgBytes, err := json.Marshal(msgMap)

			if err != nil {

				log.Printf("Error marshaling message map: %v", err)

				continue

			}

			var openAIMsg OpenAIMessage

			if err := json.Unmarshal(msgBytes, &openAIMsg); err != nil {

				log.Printf("Error unmarshaling message into OpenAIMessage: %v", err)

				continue

			}

			messages = append(messages, openAIMsg)

		}



		geminiTranslationReq := ChatCompletionRequestForGemini{

			Model:    chatReq.Model,

			Messages: messages,

			Tools:    chatReq.Tools,

			Stream:   chatReq.Stream,

		}



		// Translate the request to Gemini's native format

		geminiReq := translateOpenAItoGemini(geminiTranslationReq)

		translatedBody, err := json.Marshal(geminiReq)

		if err != nil {

			http.Error(w, "failed to marshal translated Gemini request", http.StatusInternalServerError)

			return

		}

		bodyToForward = translatedBody

	} else {

		log.Println("--> [GEMINI] Detected native Gemini API request")

		// For native requests, extract model from path and forward body as-is

		pathParts := strings.Split(r.URL.Path, "/")

		if len(pathParts) > 0 {

			modelAndAction := pathParts[len(pathParts)-1]

			modelName = strings.Split(modelAndAction, ":")[0]

		}

		bodyToForward = bodyBytes

		if strings.Contains(r.URL.Path, "stream") {

			isStreaming = true

		}

	}



	if modelName == "" {

		http.Error(w, "model name not found in request", http.StatusBadRequest)

		return

	}

	log.Printf("--> [GEMINI] Processing for model: %s, Streaming: %v", modelName, isStreaming)



	provider := getProviderByModel(modelName)

	if provider == nil {

		http.Error(w, fmt.Sprintf("unsupported model: %s", modelName), http.StatusBadRequest)

		return

	}



	// 3. Construct the target URL for the Gemini API

	var targetURL string

	if strings.Contains(r.URL.Path, "/chat/completions") {

		// Differentiate between streaming and non-streaming endpoints

		if isStreaming {

			targetURL = fmt.Sprintf("%s/models/%s:streamGenerateContent", provider.URL, modelName)

		} else {

			targetURL = fmt.Sprintf("%s/models/%s:generateContent", provider.URL, modelName)

		}

	} else {

		cleanPath := strings.TrimPrefix(r.URL.Path, "/")

		targetURL = fmt.Sprintf("%s/%s", provider.URL, cleanPath)

	}



	// 4. Create the outgoing request to the provider

	geminiAPIKey := provider.APIKey

	if geminiAPIKey == "" {

		http.Error(w, "Gemini API key not configured in server environment", http.StatusInternalServerError)

		return

	}



	proxyReq, err := http.NewRequestWithContext(

		r.Context(),

		"POST",

		targetURL,

		bytes.NewReader(bodyToForward),

	)

	if err != nil {

		http.Error(w, "request creation failed", http.StatusInternalServerError)

		return

	}



	// 5. Set headers and query parameters for authentication

	q := proxyReq.URL.Query()

	q.Add("key", geminiAPIKey)

	proxyReq.URL.RawQuery = q.Encode()

	proxyReq.Header.Set("x-goog-api-key", geminiAPIKey)

	proxyReq.Header.Set("Content-Type", "application/json")



	// 6. Log and execute the request

	log.Printf("--> [GEMINI] Forwarding request to: %s", proxyReq.URL)



	client := &http.Client{}

	proxyResp, err := client.Do(proxyReq)

	if err != nil {

		http.Error(w, "API request to provider failed", http.StatusBadGateway)

		return

	}

	defer proxyResp.Body.Close()



	log.Printf("--> [GEMINI] Upstream Response Status: %s", proxyResp.Status)
	

	// 7. Handle the response

	responseBodyBuffer := new(bytes.Buffer)

	responseReader := io.TeeReader(proxyResp.Body, responseBodyBuffer)



	if isStreaming && strings.Contains(r.URL.Path, "/chat/completions") {

		if err := translateGeminiToOpenAIStream(w, responseReader, modelName); err != nil {

			log.Printf("Error translating Gemini stream to OpenAI stream: %v", err)

			// Don't write header here as the stream function might have already

		}

	} else {

		for key, values := range proxyResp.Header {

			for _, value := range values {

				w.Header().Add(key, value)

			}

		}

		w.WriteHeader(proxyResp.StatusCode)

		io.Copy(w, responseReader)

	}



	// 8. Log the transaction

	reqLog, _ := json.Marshal(json.RawMessage(bodyBytes))

	respLog, _ := json.Marshal(json.RawMessage(responseBodyBuffer.Bytes()))



	dbEntry := &OpenAILog{

		Timestamp: time.Now(),

		UserID:    "doughznutz",

		Model:     modelName,

		Request:   reqLog,

		Response:  respLog,

	}



	if err := log_openai_event(dbEntry); err != nil {

		log.Printf("Error logging event: %v", err)

	}
}