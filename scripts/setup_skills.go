package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/hookweb?parseTime=true&tls=skip-verify"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	accountID := "fe5d65e1-6fc8-44d0-af53-98dc21d0f38f"

	// 1. Create Slack Type
	slackTypeKey := "lis::slack::7204909316::multitenant"
	slackTypeID := uuid.NewString()
	_, _ = db.Exec("INSERT IGNORE INTO webhook_types (id, account_id, type_key, plain_text_action, use_llm_fallback, created_at) VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP())",
		slackTypeID, accountID, slackTypeKey, "store_mysql", true)

	// Get actual ID if it existed
	db.QueryRow("SELECT id FROM webhook_types WHERE account_id = ? AND type_key = ?", accountID, slackTypeKey).Scan(&slackTypeID)

	// 2. Create Slack Skill
	slackSkillID := uuid.NewString()
	slackPrompt := "You are a Slack bot analyzer. Summarize the Slack message. Mention the channel name and the user who posted. If there's an @mention, highlight it."
	_, err = db.Exec("INSERT INTO webhook_skills (id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE skill_prompt = ?",
		slackSkillID, accountID, slackTypeKey, "slack-summary", slackPrompt, "slack", "store_mysql", "update_or_insert", 100, true, slackPrompt)
	if err != nil {
		log.Fatal("Slack skill failed: ", err)
	}
	fmt.Println("Slack skill setup.")

	// 3. Create Jira Type
	jiraTypeKey := "lis::jira::7204909316::multitenant"
	jiraTypeID := uuid.NewString()
	_, _ = db.Exec("INSERT IGNORE INTO webhook_types (id, account_id, type_key, plain_text_action, use_llm_fallback, created_at) VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP())",
		jiraTypeID, accountID, jiraTypeKey, "store_mysql", true)

	db.QueryRow("SELECT id FROM webhook_types WHERE account_id = ? AND type_key = ?", accountID, jiraTypeKey).Scan(&jiraTypeID)

	// 4. Create Jira Skill
	jiraSkillID := uuid.NewString()
	jiraPrompt := "You are a Jira project manager. Summarize the Jira issue update. Highlight the issue key (e.g., PROJ-123), the status change, and the assignee."
	_, err = db.Exec("INSERT INTO webhook_skills (id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE skill_prompt = ?",
		jiraSkillID, accountID, jiraTypeKey, "jira-summary", jiraPrompt, "jira", "store_mysql", "update_or_insert", 100, true, jiraPrompt)
	if err != nil {
		log.Fatal("Jira skill failed: ", err)
	}
	fmt.Println("Jira skill setup.")
}
