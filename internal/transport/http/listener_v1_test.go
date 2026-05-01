package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/service"
	"agenthook.store/internal/store"
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
	h.PublicBaseURL = ts.URL

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
	if !strings.Contains(webhookURL, acct["slug"].(string)+".") {
		t.Fatalf("expected short webhook url, got %q", webhookURL)
	}
	if created["webhook_id"] == "" {
		t.Fatal("missing webhook_id")
	}
	if created["deployment_mode"] != "single_tenant" {
		t.Fatalf("expected single_tenant, got %v", created["deployment_mode"])
	}

	payload := []byte(`{"event_id":"evt_v1_1","event_type":"message.received","message":{"text":"hello listener v1"}}`)
	u, _ := url.Parse(webhookURL)
	ingestURL := webhookURL
	if u != nil && u.Path != "" {
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(ts.URL, "http://")
		ingestURL = u.String()
	}
	resp2, err := http.Post(ingestURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected ingest status %d", resp2.StatusCode)
	}

	ingestWebhookURL, _ := created["ingest_webhook_url"].(string)
	if ingestWebhookURL == "" {
		t.Fatal("missing ingest_webhook_url")
	}
	legacyURL, _ := created["legacy_webhook_url"].(string)
	if legacyURL == "" {
		t.Fatal("missing legacy_webhook_url")
	}

	uLegacy, _ := url.Parse(ingestWebhookURL)
	if uLegacy != nil && uLegacy.Path != "" {
		uLegacy.Scheme = "http"
		uLegacy.Host = strings.TrimPrefix(ts.URL, "http://")
		ingestWebhookURL = uLegacy.String()
	}
	legacyPayload := []byte(`{"event_id":"evt_v1_2","event_type":"message.received","message":{"text":"hello listener v1 again"}}`)
	respLegacy, err := http.Post(ingestWebhookURL, "application/json", bytes.NewReader(legacyPayload))
	if err != nil {
		t.Fatal(err)
	}
	defer respLegacy.Body.Close()
	if respLegacy.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected legacy ingest status %d", respLegacy.StatusCode)
	}

	listenerID, _ := created["listener_id"].(string)
	reqSecrets, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/listeners/"+listenerID+"/secrets?provider=agentmail", nil)
	reqSecrets.Header.Set("Authorization", "Bearer "+token)
	respSecrets, err := http.DefaultClient.Do(reqSecrets)
	if err != nil {
		t.Fatal(err)
	}
	defer respSecrets.Body.Close()
	if respSecrets.StatusCode != http.StatusOK {
		t.Fatalf("unexpected secrets status %d", respSecrets.StatusCode)
	}
	var secrets []map[string]interface{}
	if err := json.NewDecoder(respSecrets.Body).Decode(&secrets); err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 listener secret, got %d", len(secrets))
	}
	if secrets[0]["webhook_url"] != webhookURL {
		t.Fatalf("expected listed webhook_url %q, got %v", webhookURL, secrets[0]["webhook_url"])
	}

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
	if len(events) != 2 {
		t.Fatalf("expected 2 listener events, got %d", len(events))
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

func TestListenerV1ManualSecretAndAliasUpdate(t *testing.T) {
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
	h.PublicBaseURL = ts.URL

	_, token, err := registerAccount(ts.URL, "techhiring@agentmail.to")
	if err != nil {
		t.Fatal(err)
	}

	updateReqBody := []byte(`{"public_alias":"ops-router"}`)
	updateReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/me", bytes.NewReader(updateReqBody))
	updateReq.Header.Set("Authorization", "Bearer "+token)
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatal(err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected update status %d", updateResp.StatusCode)
	}
	var updated map[string]interface{}
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated["public_alias"] != "ops-router" {
		t.Fatalf("expected updated alias, got %v", updated["public_alias"])
	}

	createBody := []byte(`{"provider":"generic-json","listener_id":"moble","deployment_mode":"multitenant","plain_text_action":"store_mysql","use_llm_fallback":false,"secret_value":"leadrouter_2026"}`)
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
	if webhookURL != ts.URL+"/ops-router.leadrouter_2026" {
		t.Fatalf("expected short manual secret url, got %q", webhookURL)
	}
	if created["webhook_id"] != "ops-router.leadrouter_2026@app.agenthook.store" {
		t.Fatalf("unexpected webhook id %v", created["webhook_id"])
	}

	payload := []byte(`{"event_id":"evt_manual_secret","message":"hello short route"}`)
	resp2, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected short ingest status %d", resp2.StatusCode)
	}

	dupSecretReqBody := []byte(`{"provider":"generic-json","secret_value":"leadrouter_2026"}`)
	dupSecretReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/listeners/moble/secrets", bytes.NewReader(dupSecretReqBody))
	dupSecretReq.Header.Set("Authorization", "Bearer "+token)
	dupSecretReq.Header.Set("Content-Type", "application/json")
	dupResp, err := http.DefaultClient.Do(dupSecretReq)
	if err != nil {
		t.Fatal(err)
	}
	defer dupResp.Body.Close()
	if dupResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected duplicate secret conflict, got %d", dupResp.StatusCode)
	}
}

func TestListenerV1AllowsDuplicateTypeKeysAndUsesLatestListener(t *testing.T) {
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
	h.PublicBaseURL = ts.URL

	acct, token, err := registerAccount(ts.URL, "techhiring@agentmail.to")
	if err != nil {
		t.Fatal(err)
	}

	create := func(secret string) map[string]interface{} {
		body := []byte(`{"provider":"generic-json","listener_id":"moble","deployment_mode":"multitenant","plain_text_action":"store_mysql","use_llm_fallback":false,"secret_value":"` + secret + `"}`)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/listeners", bytes.NewReader(body))
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
		return created
	}

	first := create("first-secret")
	second := create("second-secret")

	if first["type_key"] != second["type_key"] {
		t.Fatalf("expected duplicate listener type key, got %v vs %v", first["type_key"], second["type_key"])
	}
	if first["secret_id"] == second["secret_id"] {
		t.Fatal("expected second listener to create a fresh secret")
	}

	secretReqBody := []byte(`{"provider":"generic-json","secret_value":"latest-secret-bound"}`)
	secretReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/listeners/moble/secrets", bytes.NewReader(secretReqBody))
	secretReq.Header.Set("Authorization", "Bearer "+token)
	secretReq.Header.Set("Content-Type", "application/json")
	secretResp, err := http.DefaultClient.Do(secretReq)
	if err != nil {
		t.Fatal(err)
	}
	defer secretResp.Body.Close()
	if secretResp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected create secret status %d", secretResp.StatusCode)
	}

	eventsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/listeners/moble/events?provider=generic-json", nil)
	eventsReq.Header.Set("Authorization", "Bearer "+token)
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatal(err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected events status %d", eventsResp.StatusCode)
	}

	types, err := st.ListWebhookTypes(context.Background(), acct["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(types) < 2 {
		t.Fatalf("expected duplicate type rows, got %d", len(types))
	}
}

func TestShortWebhookBlockedIdentityStopsIngress(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{
		Store:    st,
		Pinecone: integrations.NewPineconeClient("", "", ""),
		LLM:      integrations.NewLLMClient("", "", "", ""),
		Executor: service.NewActionService(nil),
	}
	h := &Handler{Store: st, Processor: proc, MailDomain: "app.agenthook.store"}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()
	h.PublicBaseURL = ts.URL

	acct, token, err := registerAccount(ts.URL, "blocked@example.com")
	if err != nil {
		t.Fatal(err)
	}

	createBody := []byte(`{"provider":"generic-json","listener_id":"blockedmail","deployment_mode":"multitenant","plain_text_action":"store_mysql","use_llm_fallback":false,"secret_value":"blockme"}`)
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
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	reqIDs, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/webhook-identities", nil)
	reqIDs.Header.Set("Authorization", "Bearer "+token)
	respIDs, err := http.DefaultClient.Do(reqIDs)
	if err != nil {
		t.Fatal(err)
	}
	defer respIDs.Body.Close()
	var identities []map[string]any
	if err := json.NewDecoder(respIDs.Body).Decode(&identities); err != nil {
		t.Fatal(err)
	}
	if len(identities) == 0 {
		t.Fatal("expected webhook identity to be listed")
	}
	identityID, _ := identities[0]["id"].(string)
	blockReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/webhook-identities/"+identityID+"/block", nil)
	blockReq.Header.Set("Authorization", "Bearer "+token)
	blockResp, err := http.DefaultClient.Do(blockReq)
	if err != nil {
		t.Fatal(err)
	}
	defer blockResp.Body.Close()
	if blockResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected block status %d", blockResp.StatusCode)
	}

	webhookURL, _ := created["webhook_url"].(string)
	u, _ := url.Parse(webhookURL)
	if u != nil {
		u.Scheme = "http"
		u.Host = strings.TrimPrefix(ts.URL, "http://")
		webhookURL = u.String()
	}
	resp2, err := http.Post(webhookURL, "application/json", bytes.NewReader([]byte(`{"hello":"world"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected blocked short webhook to return 401, got %d", resp2.StatusCode)
	}

	if _, err := st.ResolveSecretAnyType(context.Background(), acct["id"].(string), "blockme"); err == nil {
		t.Fatal("expected blocked identity to invalidate secret resolution")
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
