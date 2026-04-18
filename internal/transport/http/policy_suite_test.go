package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/domain"
	"hookweb.club/internal/service"
	"hookweb.club/internal/store"
)

type policyPineconeSpy struct {
	writes     int
	lastPriorN int
}

type policyLLMStub struct{}

func (p *policyPineconeSpy) Query(_ context.Context, _ string, _ string, _ int) ([]domain.PineconeMemory, error) {
	return []domain.PineconeMemory{{ID: "existing-memory", Score: 0.98, Meta: map[string]interface{}{"type_key": "generic-json"}}}, nil
}

func (p *policyPineconeSpy) UpsertOrUpdate(_ context.Context, _ string, _ string, _ string, _ string, prior []domain.PineconeMemory) error {
	p.writes++
	p.lastPriorN = len(prior)
	return nil
}

func (p policyLLMStub) SuggestAction(_ context.Context, _ string, _ string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	return domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm-stub", Params: map[string]interface{}{}}, nil
}

func TestPolicySkillFlow_APIAndMemoryModes(t *testing.T) {
	st := store.NewMemoryStore()
	acct, token, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(context.Background(), acct.ID, "generic-json", "", true)
	_, secretRaw, _ := st.CreateSecret(context.Background(), acct.ID, wt.ID)

	pine := &policyPineconeSpy{}
	processor := &service.Processor{
		Store:    st,
		Pinecone: pine,
		LLM:      policyLLMStub{},
		Executor: service.NewActionService(nil),
	}
	h := &Handler{Store: st, Processor: processor}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	postAuth := func(path string, body string) map[string]interface{} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			var b bytes.Buffer
			_, _ = b.ReadFrom(resp.Body)
			t.Fatalf("POST %s failed: %d %s", path, resp.StatusCode, b.String())
		}
		var out map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	// 1) master prompt API
	postAuth("/api/policy/master", `{"prompt_text":"Use skill first, then default action.","updated_by":"qa"}`)
	// 2) skill API - no memory write
	postAuth("/api/policy/skills", `{"type_key":"generic-json","skill_key":"drop-heartbeat","skill_prompt":"ignore heartbeat","match_contains":"heartbeat,metrics","forced_action":"no_action","memory_write_mode":"none","priority":1,"enabled":true}`)
	// 3) skill API - insert only
	postAuth("/api/policy/skills", `{"type_key":"generic-json","skill_key":"incident-store","skill_prompt":"store incident","match_contains":"incident,sev1","forced_action":"store_mysql","memory_write_mode":"insert_only","priority":2,"enabled":true}`)

	// 4) webhook call heartbeat -> no_action and no pinecone write
	resp1, err := http.Post(ts.URL+"/url/"+acct.Slug+"/generic-json/"+secretRaw, "application/json", bytes.NewBufferString(`{"event":"heartbeat","kind":"metrics","ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusAccepted {
		t.Fatalf("webhook heartbeat status=%d", resp1.StatusCode)
	}
	var out1 map[string]interface{}
	_ = json.NewDecoder(resp1.Body).Decode(&out1)
	dec1 := out1["decision"].(map[string]interface{})
	if dec1["action_name"] != "no_action" {
		t.Fatalf("expected no_action, got %v", dec1["action_name"])
	}
	if pine.writes != 0 {
		t.Fatalf("expected no pinecone writes for memory_write_mode=none, got %d", pine.writes)
	}

	// 5) webhook call incident -> insert_only and pinecone write with empty prior
	resp2, err := http.Post(ts.URL+"/url/"+acct.Slug+"/generic-json/"+secretRaw, "application/json", bytes.NewBufferString(`{"event":"incident","sev":"sev1","id":"INC-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("webhook incident status=%d", resp2.StatusCode)
	}
	var out2 map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&out2)
	dec2 := out2["decision"].(map[string]interface{})
	if dec2["action_name"] != "store_mysql" {
		t.Fatalf("expected store_mysql, got %v", dec2["action_name"])
	}
	if pine.writes != 1 {
		t.Fatalf("expected one pinecone write, got %d", pine.writes)
	}
	if pine.lastPriorN != 0 {
		t.Fatalf("expected insert_only mode to pass zero prior, got %d", pine.lastPriorN)
	}

	// 6) list events API
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/events?limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	evResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer evResp.Body.Close()
	if evResp.StatusCode != http.StatusOK {
		t.Fatalf("list events status=%d", evResp.StatusCode)
	}
}
