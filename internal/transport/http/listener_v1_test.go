package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/integrations"
	"hookweb.club/internal/service"
	"hookweb.club/internal/store"
)

func TestListenerV1CreateIngestAndListEvents(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{
		Store:    st,
		Pinecone: integrations.NewPineconeClient("", "", ""),
		LLM:      integrations.NewLLMClient("", "", "", ""),
		Executor: service.NewActionService(nil),
	}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	acct, token, err := registerAccount(ts.URL, "techhiring@agentmail.to")
	if err != nil {
		t.Fatal(err)
	}

	createBody := []byte(`{"provider":"agentmail","deployment_mode":"enterprise","plain_text_action":"store_mysql","use_llm_fallback":false}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/listeners", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected create status %d", resp.StatusCode)
	}
	var created map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	webhookURL, _ := created["webhook_url"].(string)
	if webhookURL == "" {
		t.Fatal("missing webhook_url")
	}
	if created["deployment_mode"] != "single_tenant" {
		t.Fatalf("expected single_tenant, got %v", created["deployment_mode"])
	}

	payload := []byte(`{"event_id":"evt_v1_1","event_type":"message.received","message":{"text":"hello listener v1"}}`)
	resp2, err := http.Post(ts.URL+webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected ingest status %d", resp2.StatusCode)
	}

	listenerID, _ := created["listener_id"].(string)
	req3, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/listeners/"+listenerID+"/events?provider=agentmail", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("unexpected events status %d", resp3.StatusCode)
	}
	var events []map[string]interface{}
	if err := json.NewDecoder(resp3.Body).Decode(&events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 listener event, got %d", len(events))
	}
	if events[0]["provider"] != "agentmail" {
		t.Fatalf("unexpected provider %v", events[0]["provider"])
	}
	raw, _ := events[0]["raw_payload_json"].(string)
	processed, _ := events[0]["processed_text"].(string)
	if raw == "" || processed == "" {
		t.Fatalf("expected raw and processed text, got raw=%q processed=%q", raw, processed)
	}

	types, err := st.ListWebhookTypes(context.Background(), acct["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(types) == 0 {
		t.Fatal("expected created type")
	}
}

func registerAccount(baseURL, email string) (map[string]interface{}, string, error) {
	regBody := []byte(`{"email":"` + email + `"}`)
	resp, err := http.Post(baseURL+"/api/register/email", "application/json", bytes.NewReader(regBody))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	token, _ := out["token"].(string)
	account, _ := out["account"].(map[string]interface{})
	return account, token, nil
}
