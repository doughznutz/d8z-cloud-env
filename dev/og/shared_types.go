package main

// OpenAITool represents a tool definition in an OpenAI-compatible request.
type OpenAITool struct {
	Type     string `json:"type"`
	Function *struct { // Pointer to allow for nil
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Parameters  interface{} `json:"parameters"`
	} `json:"function"`
}

// ChatCompletionRequest is the OpenAI-compatible request structure.
type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"` // Use map[string]interface{} to handle flexible message content
	Stream   bool                     `json:"stream"`	
	Tools    []OpenAITool             `json:"tools,omitempty"`
}

// OpenAIToolCall represents a tool call from the model in a stream chunk.
type OpenAIToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// ChatCompletionStreamChunk is the OpenAI-compatible stream chunk structure.
type ChatCompletionStreamChunk struct {
	ID      string `json:"id,omitempty"`
	Object  string `json:"object,omitempty"`
	Created int64  `json:"created,omitempty"`
	Model   string `json:"model,omitempty"`
	Choices []struct {
		Delta struct {
			Content   string           `json:"content,omitempty"`
			ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"` // Pointer to allow for null value
	} `json:"choices"`
}