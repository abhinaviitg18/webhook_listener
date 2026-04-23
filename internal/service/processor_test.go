package service

import (
	"context"
	"testing"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/store"
)

type fakePinecone struct{}

func (f fakePinecone) Query(_ context.Context, _ string, _ string, _ int) ([]domain.PineconeMemory, error) {
	return []domain.PineconeMemory{{ID: "m1", Summary: "past similar alert"}}, nil
}
func (f fakePinecone) UpsertOrUpdate(_ context.Context, _ string, _ string, _ string, _ string, _ []domain.PineconeMemory) error {
	return nil
}

type capturingPinecone struct {
	queryOut   []domain.PineconeMemory
	upserted   int
	lastPriorN int
}

func (f *capturingPinecone) Query(_ context.Context, _ string, _ string, _ int) ([]domain.PineconeMemory, error) {
	if f.queryOut == nil {
		return []domain.PineconeMemory{{ID: "m1", Summary: "past similar alert"}}, nil
	}
	return f.queryOut, nil
}
func (f *capturingPinecone) UpsertOrUpdate(_ context.Context, _ string, _ string, _ string, _ string, prior []domain.PineconeMemory) error {
	f.upserted++
	f.lastPriorN = len(prior)
	return nil
}

type fakeLLM struct{ action string }

func (f fakeLLM) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	return domain.ProcessDecision{ActionName: f.action, Reason: "mock", Params: map[string]interface{}{}}, nil
}

type countingLLM struct {
	called int
	action string
}

func (c *countingLLM) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	c.called++
	return domain.ProcessDecision{ActionName: c.action, Reason: "mock", Params: map[string]interface{}{}}, nil
}

type fakeExec struct{ seen string }

func (f *fakeExec) Execute(_ context.Context, a domain.ProcessDecision, _ domain.Account, _ domain.WebhookEvent, _ []domain.ForwardTarget) error {
	f.seen = a.ActionName
	return nil
}
func (f *fakeExec) AvailableActions() []string { return []string{"forward_http", "store_mysql"} }

func TestProcessor_PlainTextActionWins(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "telegram-update", "forward_http", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	exec := &fakeExec{}
	p := &Processor{Store: st, Pinecone: fakePinecone{}, LLM: fakeLLM{action: "store_mysql"}, Executor: exec}
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r1", `{"message":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "forward_http" {
		t.Fatalf("expected plain text action, got %s", d.ActionName)
	}
}

func TestProcessor_LLMFallback(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	exec := &fakeExec{}
	p := &Processor{Store: st, Pinecone: fakePinecone{}, LLM: fakeLLM{action: "store_mysql"}, Executor: exec}
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r2", `{"message":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "store_mysql" {
		t.Fatalf("expected llm action, got %s", d.ActionName)
	}
}

func TestProcessor_SkillForcedActionAndNoMemoryWrite(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.UpsertMasterPromptPolicy(context.Background(), acct.ID, "Master policy prompt", "qa")
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "drop-noisy-heartbeats",
		SkillPrompt:     "Ignore heartbeat payloads",
		MatchContains:   "heartbeat,metrics",
		ForcedAction:    "no_action",
		MemoryWriteMode: "none",
		Priority:        1,
		Enabled:         true,
	})
	exec := &fakeExec{}
	pine := &capturingPinecone{}
	p := &Processor{Store: st, Pinecone: pine, LLM: fakeLLM{action: "forward_http"}, Executor: exec}
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r3", `{"event":"heartbeat","kind":"metrics","value":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "no_action" {
		t.Fatalf("expected skill forced action no_action, got %s", d.ActionName)
	}
	if pine.upserted != 0 {
		t.Fatalf("expected no pinecone write, got %d", pine.upserted)
	}
}

func TestProcessor_SkillInsertOnlyMemory(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "github-push", "store_mysql", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "always-insert",
		SkillPrompt:     "Always create fresh memory",
		MatchContains:   "repository,head_commit",
		ForcedAction:    "store_mysql",
		MemoryWriteMode: "insert_only",
		Priority:        1,
		Enabled:         true,
	})
	exec := &fakeExec{}
	pine := &capturingPinecone{
		queryOut: []domain.PineconeMemory{
			{ID: "existing", Score: 0.99, Meta: map[string]interface{}{"type_key": wt.TypeKey}},
		},
	}
	p := &Processor{Store: st, Pinecone: pine, LLM: fakeLLM{action: "store_mysql"}, Executor: exec}
	_, _, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r4", `{"repository":{"full_name":"org/repo"},"head_commit":{"id":"abc123"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if pine.upserted != 1 {
		t.Fatalf("expected single pinecone write, got %d", pine.upserted)
	}
	if pine.lastPriorN != 0 {
		t.Fatalf("expected insert_only to ignore prior memories, got prior=%d", pine.lastPriorN)
	}
}

func TestProcessor_LLMCanOverrideMemoryModeViaParams(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	exec := &fakeExec{}
	pine := &capturingPinecone{
		queryOut: []domain.PineconeMemory{
			{ID: "existing", Score: 0.99, Meta: map[string]interface{}{"type_key": wt.TypeKey}},
		},
	}
	llm := fakeLLMWithParams{action: "store_mysql", params: map[string]interface{}{"memory_write_mode": "none"}}
	p := &Processor{Store: st, Pinecone: pine, LLM: llm, Executor: exec}
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r5", `{"message":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "store_mysql" {
		t.Fatalf("expected store_mysql action, got %s", d.ActionName)
	}
	if pine.upserted != 0 {
		t.Fatalf("expected no pinecone write due to llm memory_write_mode none, got %d", pine.upserted)
	}
}

func TestProcessor_DedupesBySourceEventID(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "store_mysql", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	exec := &fakeExec{}
	pine := &capturingPinecone{}
	p := &Processor{Store: st, Pinecone: pine, LLM: fakeLLM{action: "store_mysql"}, Executor: exec}

	payload := `{"event_id":"evt_dup_1","event_type":"inbox.message.received","message":{"subject":"Hello"}}`
	ev1, d1, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r6", payload)
	if err != nil {
		t.Fatal(err)
	}
	ev2, d2, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r7", payload)
	if err != nil {
		t.Fatal(err)
	}
	if ev1.ID != ev2.ID {
		t.Fatalf("expected same event for duplicate source_event_id, got %s and %s", ev1.ID, ev2.ID)
	}
	if d1.ActionName != "store_mysql" {
		t.Fatalf("expected first decision store_mysql, got %s", d1.ActionName)
	}
	if d2.ActionName != "no_action" {
		t.Fatalf("expected duplicate decision no_action, got %s", d2.ActionName)
	}
	events, err := st.ListEvents(context.Background(), acct.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 stored event after duplicate processing, got %d", len(events))
	}
}

func TestProcessor_DeterministicOnlySkipsLLMFallback(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "ai-recruiter-inbox-message", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	exec := &fakeExec{}
	llm := &countingLLM{action: "forward_http"}
	p := &Processor{
		Store:             st,
		Pinecone:          fakePinecone{},
		LLM:               llm,
		Executor:          exec,
		DeterministicOnly: map[string]struct{}{"ai-recruiter-inbox-message": {}},
	}
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r8", `{"event_id":"evt1","event_type":"inbox.message.received"}`)
	if err != nil {
		t.Fatal(err)
	}
	if llm.called != 0 {
		t.Fatalf("expected llm not to be called, got %d", llm.called)
	}
	if d.ActionName != "store_mysql" {
		t.Fatalf("expected deterministic default action, got %s", d.ActionName)
	}
}

type fakeLLMWithParams struct {
	action string
	params map[string]interface{}
}

func (f fakeLLMWithParams) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	return domain.ProcessDecision{ActionName: f.action, Reason: "mock", Params: f.params}, nil
}
