package main

import (
	"context"
	"log"

	"agenthook.store/internal/config"
	"agenthook.store/internal/mail"
	"agenthook.store/internal/store"

	"github.com/aws/aws-lambda-go/lambda"
)

type sesReceiptEvent struct {
	Records []struct {
		SES struct {
			Mail struct {
				Destination []string `json:"destination"`
			} `json:"mail"`
		} `json:"ses"`
	} `json:"Records"`
}

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, event sesReceiptEvent) (map[string]string, error) {
	if err := config.LoadLambdaRuntimeEnv(ctx); err != nil {
		return nil, err
	}
	if err := config.LoadEnvFiles("local.env", ".env"); err != nil {
		return nil, err
	}
	cfg := config.Load()
	accountStore, err := store.NewMySQLStore(cfg.TiDBDSN)
	if err != nil {
		return nil, err
	}
	guard := &mail.ReceiptGuardService{
		Store:      accountStore,
		MailDomain: cfg.MailDomain,
	}
	disposition := mail.ReceiptDispositionContinue
	reason := "no_recipients"
	for _, record := range event.Records {
		for _, destination := range record.SES.Mail.Destination {
			result, evalErr := guard.EvaluateRecipient(ctx, destination)
			if evalErr != nil {
				return nil, evalErr
			}
			log.Printf("mail.receipt_guard recipient=%s disposition=%s reason=%s status=%s", destination, result.Disposition, result.Reason, result.Status)
			reason = result.Reason
			if result.Disposition == mail.ReceiptDispositionStopRuleSet {
				disposition = result.Disposition
				return map[string]string{
					"disposition": disposition,
					"reason":      reason,
				}, nil
			}
		}
	}
	return map[string]string{
		"disposition": disposition,
		"reason":      reason,
	}, nil
}
