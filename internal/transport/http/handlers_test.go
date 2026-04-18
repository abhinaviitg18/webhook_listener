package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/integrations"
	"hookweb.club/internal/service"
	"hookweb.club/internal/store"
)

func TestEndToEndLocalFlow(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{Store: st, Pinecone: integrations.NewPineconeClient("", "", "default"), LLM: integrations.NewLLMClient("", "", "", ""), Executor: service.NewActionService(nil)}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Register
	regBody := []byte(`{"email":"7204909316@agentmail.to"}`)
	resp, err := http.Post(ts.URL+"/api/register/email", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status %d", resp.StatusCode)
	}
	var reg map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	token := reg["token"].(string)

	// Create type
	typeReq := []byte(`{"type_key":"generic-json","plain_text_action":"store_mysql","use_llm_fallback":true}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhooks/types", bytes.NewReader(typeReq))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create type status %d", resp2.StatusCode)
	}

	// Create secret
	secReq := []byte(`{"type_key":"generic-json"}`)
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhooks/secrets", bytes.NewReader(secReq))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusCreated {
		t.Fatalf("create secret status %d", resp3.StatusCode)
	}
	var sec map[string]interface{}
	_ = json.NewDecoder(resp3.Body).Decode(&sec)
	webhookURL := sec["webhook_url"].(string)

	// Send webhook
	resp4, err := http.Post(ts.URL+webhookURL, "application/json", bytes.NewReader([]byte(`{"message":"hello"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusAccepted {
		t.Fatalf("webhook status %d", resp4.StatusCode)
	}

	// List events
	req5, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/events?limit=5", nil)
	req5.Header.Set("Authorization", "Bearer "+token)
	resp5, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("events status %d", resp5.StatusCode)
	}
}
