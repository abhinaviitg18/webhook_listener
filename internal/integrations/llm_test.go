package integrations

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agenthook.store/internal/domain"
)

func TestNormalizeModelAlias(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{name: "groq legacy 70b", provider: "groq", model: "llama3-70b-8192", want: "llama-3.3-70b-versatile"},
		{name: "groq legacy 8b", provider: "groq", model: "llama3-8b-8192", want: "llama-3.1-8b-instant"},
		{name: "cerebras legacy 70b", provider: "cerebras", model: "llama3.1-70b", want: "llama-3.3-70b"},
		{name: "groq current unchanged", provider: "groq", model: "llama-3.3-70b-versatile", want: "llama-3.3-70b-versatile"},
		{name: "other provider unchanged", provider: "openrouter", model: "llama3-70b-8192", want: "llama3-70b-8192"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeModelAlias(tt.provider, tt.model)
			if got != tt.want {
				t.Fatalf("normalizeModelAlias(%q, %q) = %q, want %q", tt.provider, tt.model, got, tt.want)
			}
		})
	}
}

func TestNormalizeJSONResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "raw json", input: `{"a":1}`, want: `{"a":1}`},
		{name: "json fenced", input: "```json\n{\"a\":1}\n```", want: `{"a":1}`},
		{name: "plain fenced", input: "```\n{\"a\":1}\n```", want: `{"a":1}`},
		{name: "prefixed explanation", input: "Here is the result:\n```json\n{\"a\":1}\n```", want: `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeJSONResponse(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeJSONResponse(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompactMemories(t *testing.T) {
	memories := []domain.PineconeMemory{
		{Summary: "first summary " + strings.Repeat("a", 240)},
		{Summary: "second summary"},
		{Summary: "third summary"},
		{Summary: "fourth summary should be dropped"},
	}
	got := compactMemories(memories)
	if strings.Contains(got, "fourth summary") {
		t.Fatalf("expected extra memories to be pruned, got %q", got)
	}
	if len(got) > 800 {
		t.Fatalf("expected compacted memory context <= 800 bytes, got %d", len(got))
	}
	if got == "" {
		t.Fatalf("expected non-empty memory context")
	}
}

type stubLLMClient struct {
	decision domain.ProcessDecision
	err      error
	calls    *int
}

func (s stubLLMClient) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	if s.calls != nil {
		(*s.calls)++
	}
	return s.decision, s.err
}

func TestFallbackLLMClientFallsThroughErrorsAndParseFallbacks(t *testing.T) {
	var firstCalls, secondCalls, thirdCalls int
	client := NewFallbackLLMClient(
		stubLLMClient{err: errors.New("429 Too Many Requests"), calls: &firstCalls},
		stubLLMClient{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm parse fallback"}, calls: &secondCalls},
		stubLLMClient{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "ok", ProcessedText: "summary"}, calls: &thirdCalls},
	)
	got, err := client.SuggestAction(context.Background(), "generic-json", "{}", nil, []string{"store_mysql"})
	if err != nil {
		t.Fatalf("SuggestAction returned error: %v", err)
	}
	if got.ProcessedText != "summary" {
		t.Fatalf("expected final fallback client to win, got %+v", got)
	}
	if firstCalls != 1 || secondCalls != 1 || thirdCalls != 1 {
		t.Fatalf("expected all fallback clients to be attempted once, got first=%d second=%d third=%d", firstCalls, secondCalls, thirdCalls)
	}
}

func TestFallbackLLMClientReturnsLastErrorWhenAllFail(t *testing.T) {
	client := NewFallbackLLMClient(
		stubLLMClient{err: errors.New("429 Too Many Requests")},
		stubLLMClient{err: errors.New("503 Service Unavailable")},
	)
	_, err := client.SuggestAction(context.Background(), "generic-json", "{}", nil, []string{"store_mysql"})
	if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Fatalf("expected last error to be returned, got %v", err)
	}
}

func TestShouldFallbackDecision(t *testing.T) {
	cases := []struct {
		reason string
		want   bool
	}{
		{reason: "llm not configured", want: true},
		{reason: "llm parse fallback", want: true},
		{reason: "ok", want: false},
	}
	for _, tc := range cases {
		if got := shouldFallbackDecision(domain.ProcessDecision{Reason: tc.reason}); got != tc.want {
			t.Fatalf("shouldFallbackDecision(%q) = %t, want %t", tc.reason, got, tc.want)
		}
	}
}

func TestNormalizeModelAliasLeavesCerebrasUntouched(t *testing.T) {
	got := normalizeModelAlias("cerebras", "llama3.1-8b")
	if got != "llama3.1-8b" {
		t.Fatalf("expected supported cerebras model to remain unchanged, got %q", got)
	}
}

func TestFallbackLLMClientReturnsLastDecisionWhenNoProviderSucceeds(t *testing.T) {
	client := NewFallbackLLMClient(
		stubLLMClient{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm not configured"}},
		stubLLMClient{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm parse fallback"}},
	)
	got, err := client.SuggestAction(context.Background(), "generic-json", "{}", nil, []string{"store_mysql"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Reason != "llm parse fallback" {
		t.Fatalf("expected last fallback decision, got %+v", got)
	}
}

func TestFallbackLLMClientSingleClientPassthrough(t *testing.T) {
	client := NewFallbackLLMClient(stubLLMClient{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "ok"}})
	got, err := client.SuggestAction(context.Background(), "generic-json", "{}", nil, []string{"store_mysql"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Reason != "ok" {
		t.Fatalf("expected passthrough decision, got %+v", got)
	}
}
