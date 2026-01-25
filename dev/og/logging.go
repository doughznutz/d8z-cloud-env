package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type OpenAILog struct {
	Timestamp time.Time       `json:"timestamp"`
	UserID    string          `json:"user_id"`
	Model     string          `json:"model"`
	Request   json.RawMessage `json:"request"`
	Response  json.RawMessage `json:"response"`
}

var log_openai_event = func(dbEntry *OpenAILog) error {
	logFile := fmt.Sprintf("logs/openai_logs_%s.jsonl", time.Now().Format("20060102_150405"))

	// Ensure the log directory exists.
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Marshal the whole entry to a single JSON line
	logEntryBytes, err := json.Marshal(dbEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Open the file in append mode, create if it doesn't exist
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Write the log entry followed by a newline
	if _, err := f.Write(append(logEntryBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}

	// Also print to console for debugging
	if prettyReq, err := json.MarshalIndent(json.RawMessage(dbEntry.Request), "", "  "); err == nil {
		log.Printf("OpenAI Request:\n%s", prettyReq)
	}
	if prettyResp, err := json.MarshalIndent(json.RawMessage(dbEntry.Response), "", "  "); err == nil {
		log.Printf("OpenAI Response:\n%s", prettyResp)
	}

	log.Printf("Logged OpenAI event to %s", logFile)

	return nil
}
