package integrations

import (
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
