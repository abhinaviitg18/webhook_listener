package main

import (
	"context"
	"log"

	"agenthook.store/internal/config"
	"agenthook.store/internal/mail"
	"agenthook.store/internal/store"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handle)
}

func handle(ctx context.Context, event events.S3Event) (map[string]any, error) {
	if err := config.LoadEnvFiles("local.env", ".env"); err != nil {
		return nil, err
	}
	cfg := config.Load()
	accountStore, err := store.NewMySQLStore(cfg.TiDBDSN)
	if err != nil {
		return nil, err
	}
	mailService, err := mail.NewServiceFromConfig(ctx, cfg, accountStore)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, 0, len(event.Records))
	for _, record := range event.Records {
		message, ingestErr := mailService.IngestS3Object(ctx, record.S3.Bucket.Name, record.S3.Object.Key)
		item := map[string]any{
			"bucket": record.S3.Bucket.Name,
			"key":    record.S3.Object.Key,
		}
		if ingestErr != nil {
			log.Printf("mail.ingest failed bucket=%s key=%s err=%v", record.S3.Bucket.Name, record.S3.Object.Key, ingestErr)
			item["error"] = ingestErr.Error()
		} else {
			item["message_id"] = message.ID
			item["account_id"] = message.AccountID
		}
		results = append(results, item)
	}
	return map[string]any{"results": results}, nil
}
