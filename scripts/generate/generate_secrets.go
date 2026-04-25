package main

import (
	"context"
	"fmt"
	"log"

	"agenthook.store/internal/store"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/agenthook?parseTime=true&tls=skip-verify"
	st, err := store.NewMySQLStore(dsn)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	accountID := "fe5d65e1-6fc8-44d0-af53-98dc21d0f38f"
	accountSlug := "7204909316"

	// 1. Secret for Slack
	slackTypeKey := "lis::slack::7204909316::multitenant"
	slackType, err := st.GetWebhookTypeByAccountAndKey(ctx, accountID, slackTypeKey)
	if err != nil {
		log.Fatal(err)
	}
	_, slackRaw, _ := st.CreateSecret(ctx, accountID, slackType.ID)
	fmt.Printf("SLACK URL: http://agenthook-perm-164731-alb-1352842696.us-east-1.elb.amazonaws.com/ingest/%s/slack/7204909316/%s\n", accountSlug, slackRaw)

	// 2. Secret for Jira
	jiraTypeKey := "lis::jira::7204909316::multitenant"
	jiraType, err := st.GetWebhookTypeByAccountAndKey(ctx, accountID, jiraTypeKey)
	if err != nil {
		log.Fatal(err)
	}
	_, jiraRaw, _ := st.CreateSecret(ctx, accountID, jiraType.ID)
	fmt.Printf("JIRA URL: http://agenthook-perm-164731-alb-1352842696.us-east-1.elb.amazonaws.com/ingest/%s/jira/7204909316/%s\n", accountSlug, jiraRaw)
}
