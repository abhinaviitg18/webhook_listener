package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/hookweb?parseTime=true&tls=skip-verify"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	query := "SELECT payload_json, processed_text, created_at FROM webhook_events WHERE account_id='fe5d65e1-6fc8-44d0-af53-98dc21d0f38f' ORDER BY created_at DESC LIMIT 10"
	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("--- LAST 10 MESSAGES ---")
	for rows.Next() {
		var payload []byte
		var processed sql.NullString
		var createdAt string
		if err := rows.Scan(&payload, &processed, &createdAt); err != nil {
			log.Fatal(err)
		}

		var payloadMap map[string]interface{}
		json.Unmarshal(payload, &payloadMap)
		prettyPayload, _ := json.MarshalIndent(payloadMap, "", "  ")

		fmt.Printf("\nTime: %s\n", createdAt)
		fmt.Printf("EXACT FORM:\n%s\n", string(prettyPayload))
		if processed.Valid && processed.String != "" {
			var processedMap map[string]interface{}
			err = json.Unmarshal([]byte(processed.String), &processedMap)
			if err == nil {
				prettyProcessed, _ := json.MarshalIndent(processedMap, "", "  ")
				fmt.Printf("CLEANED FORM:\n%s\n", string(prettyProcessed))
			} else {
				fmt.Printf("CLEANED FORM:\n%s\n", processed.String)
			}
		} else {
			fmt.Println("CLEANED FORM: (Not Processed)")
		}
		fmt.Println("-------------------------------------------")
	}
}
