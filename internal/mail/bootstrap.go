package mail

import (
	"context"
	"fmt"
	"log"
	"strings"

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
		Fetcher:      S3Fetcher{Client: s3.NewFromConfig(awsConfig)},
		Config: Config{
			MailDomain:            cfg.MailDomain,
			AgentHookBaseURL:      cfg.MailAgentHookBaseURL,
			AgentHookOriginSecret: cfg.MailAgentHookOriginSecret,
		},
	}
	sender, err := buildOutboundSender(cfg, sesv2.NewFromConfig(awsConfig))
	if err != nil {
		log.Printf("mail sender disabled: %v", err)
	} else {
		svc.Sender = sender
	}
	return svc, nil
}

func newStoreFromConfig(cfg config.Config) (Store, error) {
	if cfg.UseInMemoryStore {
		return NewMemoryStore(), nil
	}
	return NewMySQLStore(cfg.MailDBDSN)
}

func buildOutboundSender(cfg config.Config, sesClient *sesv2.Client) (Sender, error) {
	switch strings.TrimSpace(strings.ToLower(cfg.MailOutboundProvider)) {
	case "resend":
		if strings.TrimSpace(cfg.MailResendAPIKey) == "" {
			return nil, errorsf("MAIL_RESEND_API_KEY is required for resend outbound")
		}
		return ResendSender{
			APIKey:  cfg.MailResendAPIKey,
			BaseURL: cfg.MailResendBaseURL,
		}, nil
	case "postmark":
		if strings.TrimSpace(cfg.MailPostmarkServerToken) == "" {
			return nil, errorsf("MAIL_POSTMARK_SERVER_TOKEN is required for postmark outbound")
		}
		return PostmarkSender{
			ServerToken: cfg.MailPostmarkServerToken,
			BaseURL:     cfg.MailPostmarkBaseURL,
		}, nil
	case "smtp":
		if strings.TrimSpace(cfg.MailSMTPHost) == "" || cfg.MailSMTPPort <= 0 {
			return nil, errorsf("MAIL_SMTP_HOST and MAIL_SMTP_PORT are required for smtp outbound")
		}
		return SMTPSender{
			Host:     cfg.MailSMTPHost,
			Port:     cfg.MailSMTPPort,
			Username: cfg.MailSMTPUsername,
			Password: cfg.MailSMTPPassword,
			UseTLS:   cfg.MailSMTPUseTLS,
		}, nil
	case "zeptomail":
		if strings.TrimSpace(cfg.MailZeptoMailAPIKey) == "" {
			return nil, errorsf("MAIL_ZEPTOMAIL_API_KEY is required for zeptomail outbound")
		}
		return ZeptoMailSender{
			APIKey:  cfg.MailZeptoMailAPIKey,
			BaseURL: cfg.MailZeptoMailBaseURL,
		}, nil
	default:
		if sesClient == nil {
			return nil, errorsf("ses client not configured")
		}
		return SESSender{Client: sesClient}, nil
	}
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
