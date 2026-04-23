package main

import (
	"context"
	"fmt"
	"log"

	"hookweb.club/internal/store"
)

func main() {
	dsn := "htcadmin:u2VyDeEsqv53I2QFCLC9LmfTI23A@tcp(airecruiter-prod-mysql.c25ue0icgcva.us-east-1.rds.amazonaws.com:3306)/hookweb?parseTime=true&tls=skip-verify"
	st, err := store.NewMySQLStore(dsn)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	email := "7204909316@agentmail.to"

	// 1. Create Account
	acct, auth_token, err := st.CreateAccount(ctx, email)
	if err != nil {
		fmt.Printf("Account might already exist: %v\n", err)
		acct, err = st.GetAccountBySlug(ctx, "7204909316")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Printf("Account created. ID: %s, Auth Token: %s\n", acct.ID, auth_token)
	}

	// 2. Create Webhook Type
	typeKey := "lis::app-message::7204909316::multitenant"
	whType, err := st.CreateWebhookType(ctx, acct.ID, typeKey, "store_mysql", false)
	if err != nil {
		fmt.Printf("Type might already exist: %v\n", err)
		whType, err = st.GetWebhookTypeByAccountAndKey(ctx, acct.ID, typeKey)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Printf("Webhook type created: %s\n", whType.TypeKey)
	}

	// 3. Create Secret
	_, rawSecret, err := st.CreateSecret(ctx, acct.ID, whType.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n--- SETUP COMPLETE ---\n")
	fmt.Printf("Account: %s\n", acct.Slug)
	fmt.Printf("Provider: app-message\n")
	fmt.Printf("WebhookID: 7204909316\n")
	fmt.Printf("Secret: %s\n", rawSecret)
	fmt.Printf("ALB URL: http://hookweb-perm-164731-alb-1352842696.us-east-1.elb.amazonaws.com/ingest/%s/app-message/7204909316/%s\n", acct.Slug, rawSecret)
}
