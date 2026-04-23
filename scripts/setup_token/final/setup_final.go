package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"hookweb.club/internal/security"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/hookweb?parseTime=true&tls=skip-verify"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	accountSlug := "7204909316"
	var accountID string
	var email string
	err = db.QueryRow("SELECT id, owner_email FROM accounts WHERE slug = ?", accountSlug).Scan(&accountID, &email)
	if err != nil {
		log.Fatal("Could not find account: ", err)
	}

	// 1. Generate Token
	token, err := security.NewToken(24)
	if err != nil {
		log.Fatal(err)
	}
	hash := security.HashValue(token)

	_, err = db.Exec("INSERT INTO account_tokens(id, account_id, token_hash, created_at) VALUES(?,?,?,UTC_TIMESTAMP())",
		uuid.NewString(), accountID, hash)
	if err != nil {
		log.Fatal("Failed to insert token: ", err)
	}

	fmt.Printf("API Token created: %s\n", token)

	// 2. Set Master Policy
	prompt := "You are a recruitment assistant. Summarize incoming webhooks. Highlight user name, contact info, and the core request or OTP if present. Keep it concise."
	_, err = db.Exec("INSERT INTO master_prompt_policies (account_id, prompt_text, updated_by, updated_at, created_at) VALUES (?, ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE prompt_text = ?, updated_by = ?, updated_at = UTC_TIMESTAMP()",
		accountID, prompt, email, prompt, email)
	if err != nil {
		log.Fatal("Failed to set master policy: ", err)
	}
	fmt.Println("Master policy set.")
}
