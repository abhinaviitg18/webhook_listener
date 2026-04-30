package mail

import (
	"strings"
	"testing"
)

func TestParseRawMessage_StripsHTMLIntoTextFallback(t *testing.T) {
	raw := strings.Join([]string{
		"From: Sarah <sarah@example.com>",
		"To: ops-router.leadrouter_2026@app.agenthook.store",
		"Subject: New lead",
		"Message-ID: <msg-1@example.com>",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		"<html><body><h1>New lead</h1><p>Name: Sarah Chen</p></body></html>",
	}, "\r\n")

	parsed, err := ParseRawMessage([]byte(raw))
	if err != nil {
		t.Fatalf("ParseRawMessage: %v", err)
	}
	if parsed.HTMLBody == "" {
		t.Fatal("expected html body to be captured")
	}
	if !strings.Contains(parsed.TextBody, "Name: Sarah Chen") {
		t.Fatalf("expected stripped text fallback, got %q", parsed.TextBody)
	}
}
