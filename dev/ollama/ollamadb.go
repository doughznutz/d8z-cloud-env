package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type OpenAILog struct {
	Timestamp time.Time
	UserID    string
	Model     string
	Request   []byte
	Response  []byte
}

// Generic []byte to JSON string conversion
func byteToString(data []byte) string {
	if !json.Valid(data) {
		log.Println("Invalid JSON data")
		return ""
	}
	return string(data)
}

// Helper function to prettify JSON data
func prettifyJSON(data []byte) ([]byte, error) {
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	prettifiedData, err := json.MarshalIndent(temp, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	return prettifiedData, nil
}

func stream_into_ollama_logs(buf bytes.Buffer) ([]byte, error) {
	var openAIChatResponse OpenAIChatResponse
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := scanner.Text()
		if bytes.HasPrefix([]byte(line), []byte("data: [DONE]")) {
		} else if bytes.HasPrefix([]byte(line), []byte("data: ")) {
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
	return json.Marshal(openAIChatResponse)
}

func insert_into_ollama_logs(dbEntry *OpenAILog) error {
	// Connection details
	host := "ollamadb"
	port := 5432
	user := "doughznutz"
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := "openai_logs"

	// Use the new prettifyJSON function
	requestJSON, err := prettifyJSON(dbEntry.Request)
	if err != nil {
		return fmt.Errorf("error processing request: %w", err)
	}

	responseJSON, err := prettifyJSON(dbEntry.Response)
	if err != nil {
		return fmt.Errorf("error processing response: %w", err)
	}
	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	// Connect
	db, err := sql.Open("ollamadb", psqlInfo)
	if err != nil {
		return err
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return (err)
	}

	// Insert
	_, err = db.Exec(
		`INSERT INTO openai_logs (user_id, model, request, response)
        VALUES ($1, $2, $3, $4)`,
		dbEntry.UserID,
		dbEntry.Model,
		requestJSON,
		responseJSON,
	)
	if err != nil {
		return (err)
	}

	log.Printf("Inserted into database:\n%s\n%s\n", requestJSON, responseJSON)

	return nil
}
