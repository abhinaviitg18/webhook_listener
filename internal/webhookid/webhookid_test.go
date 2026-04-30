package webhookid

import "testing"

func TestParseEmailAddress(t *testing.T) {
	alias, secret, domain, ok := ParseEmailAddress("Ops-Router.leadrouter_2026@app.agenthook.store")
	if !ok {
		t.Fatal("expected email address to parse")
	}
	if alias != "ops-router" {
		t.Fatalf("unexpected alias %q", alias)
	}
	if secret != "leadrouter_2026" {
		t.Fatalf("unexpected secret %q", secret)
	}
	if domain != "app.agenthook.store" {
		t.Fatalf("unexpected domain %q", domain)
	}
}

func TestBuildEmailAddress(t *testing.T) {
	got := BuildEmailAddress("Abhinaviitg18", "leadrouter_2026", "APP.AGENTHOOK.STORE")
	if got != "abhinaviitg18.leadrouter_2026@app.agenthook.store" {
		t.Fatalf("unexpected email address %q", got)
	}
}
