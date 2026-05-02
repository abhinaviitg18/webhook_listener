package mail

import (
	"testing"

	"agenthook.store/internal/config"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

func TestBuildOutboundSender(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		cfg     config.Config
		wantNil bool
		wantTyp any
	}{
		{
			name:    "ses default",
			cfg:     config.Config{MailOutboundProvider: "ses"},
			wantTyp: SESSender{},
		},
		{
			name:    "resend",
			cfg:     config.Config{MailOutboundProvider: "resend", MailResendAPIKey: "rk_test", MailResendBaseURL: "https://api.resend.com"},
			wantTyp: ResendSender{},
		},
		{
			name:    "postmark",
			cfg:     config.Config{MailOutboundProvider: "postmark", MailPostmarkServerToken: "pm_test", MailPostmarkBaseURL: "https://api.postmarkapp.com"},
			wantTyp: PostmarkSender{},
		},
		{
			name:    "smtp",
			cfg:     config.Config{MailOutboundProvider: "smtp", MailSMTPHost: "smtp.example.com", MailSMTPPort: 587, MailSMTPUseTLS: true},
			wantTyp: SMTPSender{},
		},
		{
			name:    "zeptomail",
			cfg:     config.Config{MailOutboundProvider: "zeptomail", MailZeptoMailAPIKey: "zepto_test", MailZeptoMailBaseURL: "https://api.zeptomail.com/v1.1"},
			wantTyp: ZeptoMailSender{},
		},
		{
			name:    "misconfigured resend",
			cfg:     config.Config{MailOutboundProvider: "resend"},
			wantNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sender, err := buildOutboundSender(tc.cfg, &sesv2.Client{})
			if tc.wantNil {
				if err == nil {
					t.Fatalf("expected error")
				}
				if sender != nil {
					t.Fatalf("expected nil sender")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildOutboundSender: %v", err)
			}
			switch tc.wantTyp.(type) {
			case SESSender:
				if _, ok := sender.(SESSender); !ok {
					t.Fatalf("expected SESSender, got %T", sender)
				}
			case ResendSender:
				if _, ok := sender.(ResendSender); !ok {
					t.Fatalf("expected ResendSender, got %T", sender)
				}
			case PostmarkSender:
				if _, ok := sender.(PostmarkSender); !ok {
					t.Fatalf("expected PostmarkSender, got %T", sender)
				}
			case SMTPSender:
				if _, ok := sender.(SMTPSender); !ok {
					t.Fatalf("expected SMTPSender, got %T", sender)
				}
			case ZeptoMailSender:
				if _, ok := sender.(ZeptoMailSender); !ok {
					t.Fatalf("expected ZeptoMailSender, got %T", sender)
				}
			default:
				t.Fatalf("unexpected expected type")
			}
		})
	}
}
