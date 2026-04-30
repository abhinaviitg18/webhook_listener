package mail

import (
	"context"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
)

func NewServiceFromConfig(ctx context.Context, cfg config.Config, accountStore domain.Store) (*Service, error) {
	mailStore, err := newStoreFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	awsConfig, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(cfg.MailAWSRegion))
	if err != nil {
		return nil, err
	}

	svc := &Service{
		AccountStore: accountStore,
		Store:        mailStore,
		Sender:       SESSender{Client: sesv2.NewFromConfig(awsConfig)},
		Fetcher:      S3Fetcher{Client: s3.NewFromConfig(awsConfig)},
		Config: Config{
			MailDomain:            cfg.MailDomain,
			AgentHookBaseURL:      cfg.MailAgentHookBaseURL,
			AgentHookOriginSecret: cfg.MailAgentHookOriginSecret,
		},
	}
	return svc, nil
}

func newStoreFromConfig(cfg config.Config) (Store, error) {
	if cfg.UseInMemoryStore {
		return NewMemoryStore(), nil
	}
	return NewMySQLStore(cfg.MailDBDSN)
}
