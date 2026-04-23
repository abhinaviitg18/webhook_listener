package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/service"
	"agenthook.store/internal/store"
)

type agreeClassifier struct{ key string }

func (a agreeClassifier) ClassifyType(_ context.Context, _ string, _ map[string]string, _ map[string]interface{}) (domain.TypeResolution, error) {
	return domain.TypeResolution{TypeKey: a.key, Confidence: 0.9}, nil
}

type countingClassifier struct {
	calls int
	out   domain.TypeResolution
}

func (c *countingClassifier) ClassifyType(_ context.Context, _ string, _ map[string]string, _ map[string]interface{}) (domain.TypeResolution, error) {
	c.calls++
	return c.out, nil
}

func TestAutoRoute_ClassifyAndProcess(t *testing.T) {
	st := store.NewMemoryStore()
	acct, token, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "github-push", "store_mysql", true)
	_, secretRaw, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)

	_, _ = st.CreateTypeSignature(context.Background(), domain.WebhookTypeSignature{
		AccountID:           acct.ID,
		TypeKey:             "github-push",
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		RequiredKeysJSON: `[
"$.repository.full_name",
"$.head_commit.id"
]`,
		ShapeHintsJSON:  `{"$.repository":"object","$.head_commit":"object"}`,
		HeaderHintsJSON: `{}`,
		Source:          "test",
	})
	_, _ = st.CreateTransform(context.Background(), domain.WebhookTransform{AccountID: acct.ID, TypeKey: "github-push", Version: 1, Engine: "dsl", DSLText: `{"extract":{"repo":"$.repository.full_name","commit":"$.head_commit.id"}}`, Status: "active"})

	resolver := service.NewTypeResolver(st, agreeClassifier{key: "github-push"}, agreeClassifier{key: "github-push"})
	proc := &service.Processor{Store: st, Resolver: resolver, Transformer: service.NewTransformService(st), Executor: service.NewActionService(nil)}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := []byte(`{"repository":{"full_name":"org/repo"},"head_commit":{"id":"abc123"}}`)
	resp, err := http.Post(ts.URL+"/url/7204909316/"+secretRaw, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/events?limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	eResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer eResp.Body.Close()
	var events []map[string]interface{}
	_ = json.NewDecoder(eResp.Body).Decode(&events)
	if len(events) == 0 {
		t.Fatal("expected events")
	}
	if events[0]["status"] != "processed" {
		t.Fatalf("unexpected status: %v", events[0]["status"])
	}
}

func TestAutoRoute_DeterministicOnlyBypassesResolver(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "ai-recruiter-inbox-message", "", true)
	_, secretRaw, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)

	groq := &countingClassifier{out: domain.TypeResolution{TypeKey: "unknown", Confidence: 0.2}}
	cere := &countingClassifier{out: domain.TypeResolution{TypeKey: "unknown", Confidence: 0.2}}
	resolver := service.NewTypeResolver(st, groq, cere)
	proc := &service.Processor{
		Store:             st,
		Resolver:          resolver,
		Transformer:       service.NewTransformService(st),
		Executor:          service.NewActionService(nil),
		DeterministicOnly: map[string]struct{}{"ai-recruiter-inbox-message": {}},
	}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := []byte(`{"event_id":"evt_det_1","event_type":"inbox.message.received","message":{"subject":"hello"}}`)
	resp, err := http.Post(ts.URL+"/url/7204909316/"+secretRaw, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if groq.calls != 0 || cere.calls != 0 {
		t.Fatalf("expected resolver bypass for deterministic-only type, got calls groq=%d cerebras=%d", groq.calls, cere.calls)
	}
}
