package main

import (
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

func insert_into_openai_logs(dbEntry *OpenAILog) error {
	// Connection details
	host := "postgres"
	port := 5432
	user := "doughznutz"
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := "openai_logs"

	psqlInfo := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

    log.Printf("Attempting to insert into database: %v", dbEntry)

	// Connect
	db, err := sql.Open("postgres", psqlInfo)
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
        //dbEntry.Request, 
        //dbEntry.Response,
        func() string {
            b, err := json.Marshal(dbEntry.Request)
            if err != nil {
                log.Printf("Error marshaling to JSON: %v", err)
                return ""
            }
            return string(b)
        }(),
        func() string {
            b, err := json.Marshal(dbEntry.Response)
            if err != nil {
                log.Printf("Error marshaling to JSON: %v", err)
                return ""
            }
            return string(b)
        }(),        
    )
    if err != nil {
        return(err)
    }

    log.Printf("Inserted into database: %v", dbEntry)
	// Insert
    /*
	sqlStatement := `INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`
	var id int
	err = db.QueryRow(sqlStatement, "Alice", "alice@example.com").Scan(&id)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("New user ID: %d\n", id)
    */
    return nil
}






