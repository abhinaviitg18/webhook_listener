package mail

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"agenthook.store/internal/store"
)

type stubFetcher struct {
	raw []byte
}

func (s stubFetcher) Fetch(_ context.Context, _, _ string) ([]byte, error) { return s.raw, nil }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestIngestRawMessageCreatesMailboxAndForwards(t *testing.T) {
	ctx := context.Background()
	accountStore := store.NewMemoryStore()
	acct, _, err := accountStore.CreateAccount(ctx, "ops@example.com")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	whType, err := accountStore.CreateWebhookType(ctx, acct.ID, "lis::ses-mail::leadbox::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType: %v", err)
	}
	secret, err := accountStore.CreateSecretWithValue(ctx, acct.ID, whType.ID, "leadrouter_2026")
	if err != nil {
		t.Fatalf("CreateSecretWithValue: %v", err)
	}
	_ = secret

	var capturedPath string
	svc := &Service{
		AccountStore: accountStore,
		Store:        NewMemoryStore(),
		Config: Config{
			MailDomain:            "app.agenthook.store",
			AgentHookBaseURL:      "https://app.agenthook.store",
			AgentHookOriginSecret: "shared-secret",
			HTTPClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedPath = req.URL.String()
				return &http.Response{
					StatusCode: http.StatusAccepted,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			})},
		},
	}

	raw := strings.Join([]string{
		"From: Sarah <sarah@example.com>",
		"To: " + acct.PublicAlias + ".leadrouter_2026@app.agenthook.store",
		"Subject: New lead from Acme",
		"Message-ID: <msg-1@example.com>",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Name: Sarah Chen",
	}, "\r\n")

	message, err := svc.IngestRawMessage(ctx, []byte(raw), "mail-bucket", "raw/message.eml")
	if err != nil {
		t.Fatalf("IngestRawMessage: %v", err)
	}
	if message.Direction != "inbound" {
		t.Fatalf("expected inbound message, got %q", message.Direction)
	}
	if !strings.Contains(capturedPath, acct.PublicAlias+".leadrouter_2026") {
		t.Fatalf("expected short webhook path in forward url, got %q", capturedPath)
	}
}
