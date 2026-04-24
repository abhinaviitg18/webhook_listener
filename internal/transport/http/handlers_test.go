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

func TestScaleKitLoginRedirectIncludesFixedCallback(t *testing.T) {
	h := &Handler{ScaleKitBaseURL: "https://hiddentalentclub.scalekit.dev"}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/login", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitLoginRedirect(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if got := u.Query().Get("redirect_uri"); got != "https://app.agenthook.store/auth/scalekit/callback" {
		t.Fatalf("unexpected redirect_uri: %s", got)
	}
	if got := u.Query().Get("prompt"); got != "" {
		t.Fatalf("prompt should be empty for hosted login flow, got %s", got)
	}
	if !strings.Contains(u.Path, "/a/auth/login") {
		t.Fatalf("expected /a/auth/login endpoint, got %s", u.Path)
	}
}

func TestScaleKitBaseNormalizesHookwebDevToCom(t *testing.T) {
	h := &Handler{ScaleKitBaseURL: "https://hookweb.scalekit.dev"}
	got := h.scalekitBase()
	if got != "https://hookweb.scalekit.com" {
		t.Fatalf("unexpected normalized base: %s", got)
	}
}

func TestScaleKitCallbackRedirectsToAppDomain(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/callback?code=abc123", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Scheme != "https" || u.Host != "app.agenthook.store" {
		t.Fatalf("unexpected redirect target: %s", loc)
	}
	if got := u.Query().Get("code"); got != "abc123" {
		t.Fatalf("unexpected code value: %s", got)
	}
}

func TestScaleKitCallbackExchangesCodeAndIssuesLocalToken(t *testing.T) {
	mockScaleKit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"scale_access_123"}`))
		case "/userinfo":
			if got := r.Header.Get("Authorization"); got != "Bearer scale_access_123" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"techhiring@agentmail.to"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockScaleKit.Close()

	st := store.NewMemoryStore()
	h := &Handler{Store: st, ScaleKitBaseURL: mockScaleKit.URL}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/callback?code=auth_code_123", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Host != "app.agenthook.store" {
		t.Fatalf("unexpected redirect host: %s", u.Host)
	}
	localToken := u.Query().Get("code")
	if localToken == "" {
		t.Fatalf("expected local token in code query param")
	}
	if localToken == "auth_code_123" {
		t.Fatalf("expected exchanged local token, got raw code")
	}
	acct, err := st.GetAccountByToken(context.Background(), localToken)
	if err != nil {
		t.Fatalf("expected local token to resolve account: %v", err)
	}
	if acct.OwnerEmail != "techhiring@agentmail.to" {
		t.Fatalf("unexpected account email: %s", acct.OwnerEmail)
	}
}

func TestScaleKitCallbackExchangeFailureReturnsAuthError(t *testing.T) {
	mockScaleKit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockScaleKit.Close()

	st := store.NewMemoryStore()
	h := &Handler{Store: st, ScaleKitBaseURL: mockScaleKit.URL}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/callback?code=bad_code", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Query().Get("auth_error") == "" {
		t.Fatalf("expected auth_error query param on exchange failure")
	}
	if got := u.Query().Get("code"); got != "" {
		t.Fatalf("expected no raw code passthrough on exchange failure, got %s", got)
	}
}

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
