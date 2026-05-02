package mail

import (
	"context"
	"errors"
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

type stubSender struct {
	sesMessageID string
	rfcMessageID string
	err          error
}

func (s stubSender) Send(_ context.Context, _ Mailbox, _ SendRequest, _ *Message) (string, string, error) {
	return s.sesMessageID, s.rfcMessageID, s.err
}

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

func TestSendMessageAndReply(t *testing.T) {
	ctx := context.Background()
	accountStore := store.NewMemoryStore()
	acct, _, err := accountStore.CreateAccount(ctx, "ops@example.com")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	whType, err := accountStore.CreateWebhookType(ctx, acct.ID, "lis::ses-mail::sendbox::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType: %v", err)
	}
	secret, err := accountStore.CreateSecretWithValue(ctx, acct.ID, whType.ID, "sendrouter_2026")
	if err != nil {
		t.Fatalf("CreateSecretWithValue: %v", err)
	}

	svc := &Service{
		AccountStore: accountStore,
		Store:        NewMemoryStore(),
		Sender:       stubSender{sesMessageID: "provider-msg-1", rfcMessageID: "rfc-msg-1"},
		Config:       Config{MailDomain: "app.agenthook.store"},
	}
	mailboxes, err := svc.SyncMailboxes(ctx, acct.ID)
	if err != nil {
		t.Fatalf("SyncMailboxes: %v", err)
	}
	if len(mailboxes) != 1 {
		t.Fatalf("expected 1 mailbox, got %d", len(mailboxes))
	}
	mailboxID := mailboxes[0].ID
	if mailboxes[0].SecretID != secret.ID {
		t.Fatalf("expected mailbox to use created secret")
	}

	sent, err := svc.SendMessage(ctx, acct.ID, mailboxID, SendRequest{
		To:          []string{"dest@example.com"},
		Subject:     "Send test",
		TextBody:    "Plain text body",
		HTMLBody:    "<p>Plain text body</p>",
		Attachments: []OutgoingAttachment{{FileName: "hello.txt", ContentType: "text/plain", ContentBase64: "aGVsbG8="}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sent.Direction != "outbound" || sent.SESMessageID != "provider-msg-1" {
		t.Fatalf("unexpected outbound send result: %+v", sent)
	}

	inbound, _, err := svc.Store.CreateMessage(ctx, Message{
		AccountID:    acct.ID,
		MailboxID:    mailboxID,
		ThreadID:     sent.ThreadID,
		Direction:    "inbound",
		RFCMessageID: "incoming-msg-1",
		References:   []string{"older-msg"},
		From:         []string{"sender@example.com"},
		To:           []string{mailboxes[0].EmailAddress},
		Subject:      "Original subject",
		TextBody:     "reply target",
		ParsedStatus: "parsed",
	}, nil)
	if err != nil {
		t.Fatalf("CreateMessage inbound: %v", err)
	}

	svc.Sender = stubSender{sesMessageID: "provider-msg-2", rfcMessageID: "rfc-msg-2"}
	reply, err := svc.ReplyToMessage(ctx, acct.ID, inbound.ID, SendRequest{
		TextBody: "reply body",
	})
	if err != nil {
		t.Fatalf("ReplyToMessage: %v", err)
	}
	if reply.InReplyTo != inbound.ID {
		t.Fatalf("expected reply to reference message id, got %q", reply.InReplyTo)
	}
	if reply.Subject != "Re: Original subject" {
		t.Fatalf("expected reply subject, got %q", reply.Subject)
	}
	if len(reply.To) != 1 || reply.To[0] != "sender@example.com" {
		t.Fatalf("expected reply recipient to auto-fill original sender, got %+v", reply.To)
	}
}

func TestSendMessageProviderFailureDoesNotPersistOutboundRow(t *testing.T) {
	ctx := context.Background()
	accountStore := store.NewMemoryStore()
	acct, _, err := accountStore.CreateAccount(ctx, "ops@example.com")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	whType, err := accountStore.CreateWebhookType(ctx, acct.ID, "lis::ses-mail::failbox::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType: %v", err)
	}
	if _, err := accountStore.CreateSecretWithValue(ctx, acct.ID, whType.ID, "failrouter_2026"); err != nil {
		t.Fatalf("CreateSecretWithValue: %v", err)
	}

	svc := &Service{
		AccountStore: accountStore,
		Store:        NewMemoryStore(),
		Sender:       stubSender{err: errors.New("provider exploded")},
		Config:       Config{MailDomain: "app.agenthook.store"},
	}
	mailboxes, err := svc.SyncMailboxes(ctx, acct.ID)
	if err != nil {
		t.Fatalf("SyncMailboxes: %v", err)
	}
	_, err = svc.SendMessage(ctx, acct.ID, mailboxes[0].ID, SendRequest{
		To:       []string{"dest@example.com"},
		Subject:  "Should fail",
		TextBody: "body",
	})
	if err == nil {
		t.Fatalf("expected send failure")
	}
	items, err := svc.Store.ListMessages(ctx, acct.ID, mailboxes[0].ID, 50)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no outbound rows after failed send, got %d", len(items))
	}
}
