package observability

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewLangfuseClientDisabledReturnsNoop(t *testing.T) {
	client := NewLangfuseClient(Config{})
	trace := client.StartLLMDecision(t.Context(), LLMDecisionMetadata{EventID: "evt"})
	trace.StartAttempt("groq", "model").Finish(LLMAttemptResult{Provider: "groq", Model: "model", Outcome: "success"})
	trace.Finish(LLMDecisionResult{Outcome: "success"})
}

func TestLangfuseFlushMetadataOnly(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "" {
			t.Fatalf("expected authorization header")
		}
		if got := r.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte("pk:test-sk")) {
			t.Fatalf("unexpected auth header: %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"successes":[],"errors":[]}`))
	}))
	defer server.Close()

	client := NewLangfuseClient(Config{
		Enabled:   true,
		Host:      server.URL,
		PublicKey: "pk",
		SecretKey: "test-sk",
	})
	trace := client.StartLLMDecision(t.Context(), LLMDecisionMetadata{
		EventID:               "evt_123",
		AccountHash:           "acct_hash",
		TypeKey:               "lis::github::wh::multitenant",
		Operation:             "reprocess",
		MatchedSkillKey:       "devops_workflow_monitor",
		PayloadHash:           "payload_hash",
		PayloadBytes:          21049,
		CompactedPayloadBytes: 2048,
		UsedCompaction:        true,
		FallbackChainSize:     3,
	})
	attempt := trace.StartAttempt("openrouter", "openrouter/free")
	attempt.Finish(LLMAttemptResult{
		Provider:         "openrouter",
		Model:            "openrouter/free",
		Outcome:          "success",
		StatusMessage:    "ok",
		PromptTokens:     11,
		CompletionTokens: 7,
		TotalTokens:      18,
	})
	trace.Finish(LLMDecisionResult{
		FinalAction:         "store_mysql",
		DecisionReason:      "skill:devops_workflow_monitor",
		WinningProvider:     "openrouter",
		WinningModel:        "openrouter/free",
		Outcome:             "success",
		UsedFallback:        true,
		ProducedTags:        true,
		ProcessedTextSource: "llm",
		TagsCount:           3,
	})

	batch, ok := received["batch"].([]any)
	if !ok || len(batch) < 4 {
		t.Fatalf("expected batch events, got %#v", received["batch"])
	}
	raw, _ := json.Marshal(received)
	text := string(raw)
	for _, forbidden := range []string{"Payload:", "raw_payload_json", "Relevant context", "\"input\":", "\"output\":"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected metadata-only payload without %q: %s", forbidden, text)
		}
	}
}
