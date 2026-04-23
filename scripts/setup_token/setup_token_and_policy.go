package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/agenthook?parseTime=true&tls=skip-verify"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	accountID := "fe5d65e1-6fc8-44d0-af53-98dc21d0f38f"

	// Create a simple token creation logic (since I don't want to re-implement the hashing here exactly as the app does,
	// I'll check if there's a simpler way or just use a SQL query if I know the hash format)
	// Actually, the app uses auth.GenerateToken and hashes it.

	// Better: I'll check some existing scripts to see how tokens are handled or if I can just use the one from CreateAccount.
	// The user said "create a token", so I'll create a new one in account_tokens table.

	// Let's assume the app's CreateAccount logic.
	// I'll just look for any existing tokens for this account first.

	var email string
	err = db.QueryRow("SELECT owner_email FROM accounts WHERE id = ?", accountID).Scan(&email)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Account email: %s\n", email)

	// Set Master Policy
	prompt := "You are a recruitment assistant. Summarize incoming webhooks. Highlight user name, contact info, and the core request or OTP if present. Keep it concise."
	_, err = db.Exec("INSERT INTO master_prompt_policies (account_id, prompt_text, updated_by, updated_at, created_at) VALUES (?, ?, ?, NOW(), NOW()) ON DUPLICATE KEY UPDATE prompt_text = ?, updated_by = ?, updated_at = NOW()",
		accountID, prompt, email, prompt, email)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Master policy set.")

	// For the token, I'll just grab the most recent one or create a dummy one if I can't hash.
	// Wait, I should probably check how the app hashes tokens.
}
