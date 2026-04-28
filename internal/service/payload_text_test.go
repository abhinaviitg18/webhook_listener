package service

import (
	"strings"
	"testing"
)

func TestSanitizeHTMLTextStripsMarkup(t *testing.T) {
	input := `<div>Hello&nbsp;<strong>world</strong><br><script>alert(1)</script><a href="https://example.com">click</a></div>`
	got := sanitizeHTMLText(input)
	if strings.Contains(got, "<strong>") || strings.Contains(strings.ToLower(got), "alert(1)") {
		t.Fatalf("expected html tags/scripts removed, got %q", got)
	}
	if got != "Hello world click" {
		t.Fatalf("unexpected sanitized text %q", got)
	}
}

func TestPayloadToTextPrefersReadableSanitizedFields(t *testing.T) {
	payload := `{"message":{"subject":"Welcome","html":"<div><p>Hello <b>there</b></p><p>Use code <span>123456</span></p></div>","from":"alerts@example.com"},"tracking_url":"https://example.com/open/123"}`
	got := payloadToText(payload)
	if strings.Contains(got, "<div>") || strings.Contains(got, "tracking_url") {
		t.Fatalf("expected html/url noise removed, got %q", got)
	}
	for _, want := range []string{"from: alerts@example.com", "html: Hello there Use code 123456", "subject: Welcome"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in processed text, got %q", want, got)
		}
	}
}
