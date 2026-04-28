package app

import (
	"testing"
	"time"

	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
)

func TestBuildFallbackLLMClientPrefersDefaultBYOKThenGlobals(t *testing.T) {
	cfg := config.Config{
		LLMProvider:       "openrouter",
		LLMAPIKey:         "global-openrouter-key",
		LLMBaseURL:        "https://openrouter.ai/api/v1",
		LLMModel:          "openrouter/free",
		GroqAPIKey:        "global-groq-key",
		GroqBaseURL:       "https://api.groq.com/openai/v1",
		GroqModel:         "llama3-70b-8192",
		CerebrasAPIKey:    "global-cerebras-key",
		CerebrasBaseURL:   "https://api.cerebras.ai/v1",
		CerebrasModel:     "llama3.1-70b",
		OpenRouterAPIKey:  "global-openrouter-key",
		OpenRouterBaseURL: "https://openrouter.ai/api/v1",
		OpenRouterModel:   "openrouter/free",
	}
	now := time.Now().UTC()
	client := buildFallbackLLMClient([]domain.BYOKProviderConfig{
		{Provider: "cerebras", APIKey: "acct-cerebras-key", BaseURL: "https://api.cerebras.ai/v1", Model: "llama3.1-70b", CreatedAt: now.Add(1 * time.Minute)},
		{Provider: "groq", APIKey: "acct-groq-key", BaseURL: "https://api.groq.com/openai/v1", Model: "llama3-70b-8192", IsDefault: true, CreatedAt: now},
	}, cfg)

	fallback, ok := client.(*integrations.FallbackLLMClient)
	if !ok {
		t.Fatalf("expected fallback client, got %T", client)
	}
	if len(fallback.Clients) != 5 {
		t.Fatalf("expected 5 unique clients, got %d", len(fallback.Clients))
	}
	first, ok := fallback.Clients[0].(*integrations.LLMClient)
	if !ok || first.Provider != "groq" || first.APIKey != "acct-groq-key" {
		t.Fatalf("expected default BYOK groq first, got %#v", fallback.Clients[0])
	}
	second, ok := fallback.Clients[1].(*integrations.LLMClient)
	if !ok || second.Provider != "cerebras" || second.APIKey != "acct-cerebras-key" {
		t.Fatalf("expected second BYOK client next, got %#v", fallback.Clients[1])
	}
	third, ok := fallback.Clients[2].(*integrations.LLMClient)
	if !ok || third.Provider != "openrouter" || third.APIKey != "global-openrouter-key" || third.Model != "openrouter/free" {
		t.Fatalf("expected global primary after BYOK configs, got %#v", fallback.Clients[2])
	}
	fourth, ok := fallback.Clients[3].(*integrations.LLMClient)
	if !ok || fourth.Provider != "groq" || fourth.APIKey != "global-groq-key" {
		t.Fatalf("expected additional global provider fallback, got %#v", fallback.Clients[3])
	}
	fifth, ok := fallback.Clients[4].(*integrations.LLMClient)
	if !ok || fifth.Provider != "cerebras" || fifth.APIKey != "global-cerebras-key" {
		t.Fatalf("expected global cerebras fallback last, got %#v", fallback.Clients[4])
	}
}
