package service

import (
	"context"
	"encoding/json"
	"strings"
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
	lastQuery  string
	lastUpsert string
}

func (f *capturingPinecone) Query(_ context.Context, _ string, payload string, _ int) ([]domain.PineconeMemory, error) {
	f.lastQuery = payload
	if f.queryOut == nil {
		return []domain.PineconeMemory{{ID: "m1", Summary: "past similar alert"}}, nil
	}
	return f.queryOut, nil
}
func (f *capturingPinecone) UpsertOrUpdate(_ context.Context, _ string, _ string, _ string, payload string, prior []domain.PineconeMemory) error {
	f.upserted++
	f.lastPriorN = len(prior)
	f.lastUpsert = payload
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

type captureLLM struct {
	payloads []string
	decision domain.ProcessDecision
}

func (c *captureLLM) SuggestAction(_ context.Context, _ string, payload string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	c.payloads = append(c.payloads, payload)
	return c.decision, nil
}

type fakeExec struct{ seen string }

func (f *fakeExec) Execute(_ context.Context, a domain.ProcessDecision, _ domain.Account, _ domain.WebhookEvent, _ []domain.ForwardTarget) error {
	f.seen = a.ActionName
	return nil
}
func (f *fakeExec) AvailableActions() []string { return []string{"forward_http", "store_mysql"} }

type stagedLLM struct {
	calls []string
}

func (s *stagedLLM) SuggestAction(_ context.Context, typeKey, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	s.calls = append(s.calls, typeKey)
	if strings.HasPrefix(typeKey, "route:") {
		return domain.ProcessDecision{
			ActionName: "sales_lead_router",
			Reason:     "router",
			Params: map[string]interface{}{
				"spam_label":             "not_spam",
				"skill_candidates":       []string{"sales_lead_router"},
				"candidate_action":       "crm_upsert",
				"integration_target_key": "hubspot_primary",
			},
		}, nil
	}
	return domain.ProcessDecision{
		ActionName:    "crm_upsert",
		Reason:        "lead detected",
		ProcessedText: "New enterprise lead from Sarah Chen at Acme",
		Tags:          []string{"lead", "sales"},
		Params: map[string]interface{}{
			"integration_target_key": "hubspot_primary",
			"entity_payload": map[string]interface{}{
				"name":    "Sarah Chen",
				"company": "Acme",
			},
		},
	}, nil
}

type invalidIntegrationLLM struct{}

func (i invalidIntegrationLLM) SuggestAction(_ context.Context, typeKey, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	if strings.HasPrefix(typeKey, "route:") {
		return domain.ProcessDecision{
			ActionName: "sales_lead_router",
			Reason:     "router",
			Params: map[string]interface{}{
				"spam_label":             "not_spam",
				"skill_candidates":       []string{"sales_lead_router"},
				"candidate_action":       "crm_upsert",
				"integration_target_key": "hubspot_primary",
			},
		}, nil
	}
	return domain.ProcessDecision{
		ActionName:    "crm_upsert",
		Reason:        "lead detected",
		ProcessedText: "Lead needs review",
		Params: map[string]interface{}{
			"integration_target_key": "hubspot_primary",
		},
	}, nil
}

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

func TestProcessor_CompactsOversizedPayloadForLLMOnly(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "github-workflow", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	llm := &captureLLM{
		decision: domain.ProcessDecision{
			ActionName:    "store_mysql",
			Reason:        "mock",
			ProcessedText: "summarized workflow result",
			Tags:          []string{"ci", "workflow"},
			Params:        map[string]interface{}{},
		},
	}
	exec := &fakeExec{}
	p := &Processor{
		Store:    st,
		Pinecone: fakePinecone{},
		LLM:      llm,
		Executor: exec,
		LLMCompaction: LLMCompactionConfig{
			Enabled:         true,
			ThresholdBytes:  500,
			MaxStringBytes:  120,
			MaxArrayItems:   3,
			MaxObjectFields: 6,
		},
	}
	payload := `{"workflow":"Deploy AWS Lambda","repository":{"full_name":"abhinaviitg18/webhook_listener"},"jobs":[{"name":"build"},{"name":"test"},{"name":"deploy"},{"name":"cleanup"}],"logs":"` + strings.Repeat("x", 5000) + `"}`
	ev, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r9", payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(llm.payloads) != 1 {
		t.Fatalf("expected exactly one llm call, got %d", len(llm.payloads))
	}
	if !strings.Contains(llm.payloads[0], `"_compaction"`) {
		t.Fatalf("expected llm payload to include compaction metadata")
	}
	if strings.Contains(llm.payloads[0], strings.Repeat("x", 1000)) {
		t.Fatalf("expected long raw payload text to be trimmed before llm call")
	}
	stored, err := st.GetEvent(context.Background(), acct.ID, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PayloadJSON != payload {
		t.Fatalf("expected full payload to remain stored unchanged")
	}
	if stored.ProcessedText != "summarized workflow result" {
		t.Fatalf("expected processed text to persist, got %q", stored.ProcessedText)
	}
	if stored.TagsJSON == "" {
		t.Fatalf("expected tags json to persist")
	}
	if d.ProcessedText != "summarized workflow result" {
		t.Fatalf("expected llm processed text to propagate, got %q", d.ProcessedText)
	}
	if ev.ProcessedText != "summarized workflow result" {
		t.Fatalf("expected event processed text to use llm summary, got %q", ev.ProcessedText)
	}
	if ev.TagsJSON == "" {
		t.Fatalf("expected llm tags to be stored on returned event")
	}
}

func TestProcessor_SmallPayloadBypassesCompaction(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	llm := &captureLLM{decision: domain.ProcessDecision{ActionName: "store_mysql", Reason: "mock", Params: map[string]interface{}{}}}
	p := &Processor{
		Store:    st,
		Pinecone: fakePinecone{},
		LLM:      llm,
		Executor: &fakeExec{},
		LLMCompaction: LLMCompactionConfig{
			Enabled:        true,
			ThresholdBytes: 2048,
		},
	}
	payload := `{"message":"short body"}`
	if _, _, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r10", payload); err != nil {
		t.Fatal(err)
	}
	if len(llm.payloads) != 1 {
		t.Fatalf("expected one llm call, got %d", len(llm.payloads))
	}
	if llm.payloads[0] != payload {
		t.Fatalf("expected small payload to bypass compaction, got %q", llm.payloads[0])
	}
}

func TestProcessor_SkillForcedActionKeepsTagsWithCompactedPayload(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "github-workflow", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "devops_workflow_monitor",
		SkillPrompt:     "Summarize workflow result",
		MatchContains:   "workflow,deploy",
		ForcedAction:    "store_mysql",
		MemoryWriteMode: "update_or_insert",
		Priority:        1,
		Enabled:         true,
	})
	llm := &captureLLM{
		decision: domain.ProcessDecision{
			ActionName:    "forward_http",
			Reason:        "mock",
			ProcessedText: "workflow deployed successfully",
			Tags:          []string{"workflow", "success"},
			Params:        map[string]interface{}{},
		},
	}
	exec := &fakeExec{}
	p := &Processor{
		Store:    st,
		Pinecone: fakePinecone{},
		LLM:      llm,
		Executor: exec,
		LLMCompaction: LLMCompactionConfig{
			Enabled:         true,
			ThresholdBytes:  300,
			MaxStringBytes:  100,
			MaxArrayItems:   3,
			MaxObjectFields: 6,
		},
	}
	payload := `{"workflow":"deploy","repository":"webhook_listener","details":"` + strings.Repeat("signal ", 500) + `"}`
	ev, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r11", payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "store_mysql" {
		t.Fatalf("expected skill forced action to win, got %s", d.ActionName)
	}
	if ev.ProcessedText != "workflow deployed successfully" {
		t.Fatalf("expected llm summary to be preserved on event, got %q", ev.ProcessedText)
	}
	if ev.TagsJSON == "" {
		t.Fatalf("expected tags json to be stored")
	}
	var tags []string
	if err := json.Unmarshal([]byte(ev.TagsJSON), &tags); err != nil {
		t.Fatalf("expected valid tags json: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected llm tags to be preserved, got %v", tags)
	}
	stored, err := st.GetEvent(context.Background(), acct.ID, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProcessedText != "workflow deployed successfully" {
		t.Fatalf("expected processed text to persist, got %q", stored.ProcessedText)
	}
	if stored.TagsJSON == "" {
		t.Fatalf("expected persisted tags json")
	}
	if len(llm.payloads) != 1 {
		t.Fatalf("expected exactly one llm call, got %d", len(llm.payloads))
	}
	var wrapped map[string]interface{}
	if err := json.Unmarshal([]byte(llm.payloads[0]), &wrapped); err != nil {
		t.Fatalf("expected policy-aware llm payload to be valid json: %v", err)
	}
	compactedPayload, _ := wrapped["payload"].(string)
	if !strings.Contains(compactedPayload, `"_compaction"`) {
		t.Fatalf("expected compacted payload to be embedded in policy wrapper")
	}
}

func TestProcessor_StripsHTMLBeforeMatchingAndFallbackText(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", false)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "otp-mail",
		MatchContains:   "verification code",
		ForcedAction:    "no_action",
		MemoryWriteMode: "none",
		Priority:        1,
		Enabled:         true,
	})
	pine := &capturingPinecone{}
	p := &Processor{Store: st, Pinecone: pine, LLM: &countingLLM{action: "store_mysql"}, Executor: &fakeExec{}}
	payload := `{"message":{"subject":"Your code","html":"<div><p>Your <strong>verification code</strong> is <span>123456</span>.</p></div>"}}`
	ev, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r12", payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "no_action" {
		t.Fatalf("expected sanitized html text to trigger skill match, got %s", d.ActionName)
	}
	if strings.Contains(ev.ProcessedText, "<div>") {
		t.Fatalf("expected processed text to strip html, got %q", ev.ProcessedText)
	}
	if !strings.Contains(ev.ProcessedText, "verification code") {
		t.Fatalf("expected processed text to keep readable content, got %q", ev.ProcessedText)
	}
	if strings.Contains(pine.lastQuery, "<strong>") || !strings.Contains(pine.lastQuery, "verification code") {
		t.Fatalf("expected pinecone query payload to be sanitized, got %q", pine.lastQuery)
	}
}

func TestProcessor_RouterSelectsSkillAndStructuredIntegrationAction(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "marketing_noise_filter",
		SkillPrompt:     "Ignore promotional mail",
		MatchContains:   "unsubscribe,offer",
		ForcedAction:    "no_action",
		MemoryWriteMode: "none",
		Priority:        50,
		Enabled:         true,
	})
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "sales_lead_router",
		SkillPrompt:     "Route enterprise sales leads to CRM",
		MatchContains:   "enterprise,demo",
		MemoryWriteMode: "update_or_insert",
		Priority:        10,
		Enabled:         true,
	})
	_, _ = st.CreateForwardTarget(context.Background(), acct.ID, "http", BuildIntegrationTargetConfig("hubspot_primary", "Primary CRM", true, []string{"crm_upsert"}, nil, map[string]interface{}{"url": "https://example.com/hubspot"}))
	llm := &stagedLLM{}
	exec := &fakeExec{}
	p := &Processor{Store: st, Pinecone: fakePinecone{}, LLM: llm, Executor: exec}
	payload := `{"subject":"New enterprise lead from Acme Corp","message":{"text":"Name: Sarah Chen. Looking for a hiring automation demo. unsubscribe footer included."}}`
	ev, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r13", payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "crm_upsert" {
		t.Fatalf("expected crm_upsert action, got %s", d.ActionName)
	}
	if exec.seen != "crm_upsert" {
		t.Fatalf("expected executor to receive crm_upsert, got %s", exec.seen)
	}
	if len(llm.calls) != 2 || !strings.HasPrefix(llm.calls[0], "route:") {
		t.Fatalf("expected route and final llm calls, got %v", llm.calls)
	}
	if !strings.Contains(ev.ProcessedText, "Sarah Chen") {
		t.Fatalf("expected processed text to be stored, got %q", ev.ProcessedText)
	}
}

func TestProcessor_InvalidStructuredIntegrationFallsBackToManualReview(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	sec, _, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)
	_, _ = st.CreateWebhookSkill(context.Background(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         wt.TypeKey,
		SkillKey:        "sales_lead_router",
		SkillPrompt:     "Route enterprise sales leads to CRM",
		MatchContains:   "enterprise,demo",
		MemoryWriteMode: "update_or_insert",
		Priority:        10,
		Enabled:         true,
	})
	_, _ = st.CreateForwardTarget(context.Background(), acct.ID, "http", BuildIntegrationTargetConfig("hubspot_primary", "Primary CRM", true, []string{"crm_upsert"}, nil, map[string]interface{}{"url": "https://example.com/hubspot"}))
	exec := &fakeExec{}
	p := &Processor{Store: st, Pinecone: fakePinecone{}, LLM: invalidIntegrationLLM{}, Executor: exec}
	payload := `{"subject":"New enterprise lead from Acme Corp","message":{"text":"Need a hiring automation demo"}}`
	_, d, err := p.ProcessWebhook(context.Background(), acct, wt, sec, "r14", payload)
	if err != nil {
		t.Fatal(err)
	}
	if d.ActionName != "manual_review" {
		t.Fatalf("expected invalid integration action to fall back to manual_review, got %s", d.ActionName)
	}
	if exec.seen != "manual_review" {
		t.Fatalf("expected executor to see manual_review, got %s", exec.seen)
	}
}

type fakeLLMWithParams struct {
	action string
	params map[string]interface{}
}

func (f fakeLLMWithParams) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	return domain.ProcessDecision{ActionName: f.action, Reason: "mock", Params: f.params}, nil
}
