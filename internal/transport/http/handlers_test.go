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
	"time"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/security"
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

func TestScaleKitStartUsesOAuthAuthorizeWhenClientConfigured(t *testing.T) {
	h := &Handler{
		ScaleKitBaseURL:      "https://hookweb.scalekit.com",
		ScaleKitClientID:     "client_123",
		ScaleKitClientSecret: "secret_123",
		AppSessionSecret:     "session_secret",
		PublicBaseURL:        "https://app.agenthook.store",
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/start?return_to=%2Fsettings&login_hint=techhiring%40agentmail.to", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitStart(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	u, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Path != "/oauth/authorize" {
		t.Fatalf("expected /oauth/authorize endpoint, got %s", u.Path)
	}
	if got := u.Query().Get("client_id"); got != "client_123" {
		t.Fatalf("unexpected client_id: %s", got)
	}
	if got := u.Query().Get("redirect_uri"); got != "https://app.agenthook.store/auth/scalekit/callback" {
		t.Fatalf("unexpected redirect_uri: %s", got)
	}
	if got := u.Query().Get("login_hint"); got != "techhiring@agentmail.to" {
		t.Fatalf("unexpected login_hint: %s", got)
	}
	if got := u.Query().Get("state"); got == "" {
		t.Fatalf("expected signed state")
	}
}

func TestScaleKitStartFallsBackToHostedLoginWhenOAuthStateMintFails(t *testing.T) {
	h := &Handler{
		ScaleKitBaseURL:      "https://hookweb.scalekit.com",
		ScaleKitClientID:     "client_123",
		ScaleKitClientSecret: "secret_123",
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/start", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitStart(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	u, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Path != "/a/auth/login" {
		t.Fatalf("expected hosted login path, got %s", u.Path)
	}
	if got := u.Query().Get("redirect_uri"); got != "https://app.agenthook.store/auth/scalekit/callback" {
		t.Fatalf("unexpected redirect_uri: %s", got)
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
	if got := u.Query().Get("code"); got != "" {
		t.Fatalf("expected callback redirect without code query param, got %s", got)
	}
	sessionCookie := rr.Result().Cookies()
	if len(sessionCookie) == 0 {
		t.Fatalf("expected session cookie to be issued")
	}
	var localToken string
	for _, c := range sessionCookie {
		if c.Name == "htc_token" {
			localToken = c.Value
		}
	}
	if localToken == "" {
		t.Fatalf("expected htc_token cookie to be set")
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

func TestScaleKitCallbackUsesSignedStateReturnTo(t *testing.T) {
	mockScaleKit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"scale_access_123"}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"email":"techhiring@agentmail.to"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockScaleKit.Close()

	st := store.NewMemoryStore()
	h := &Handler{
		Store:                st,
		ScaleKitBaseURL:      mockScaleKit.URL,
		ScaleKitClientID:     "client_123",
		ScaleKitClientSecret: "secret_123",
		AppSessionSecret:     "session_secret",
		PublicBaseURL:        "https://app.agenthook.store",
	}
	state, err := h.mintScaleKitState("/settings")
	if err != nil {
		t.Fatalf("mint state: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/scalekit/callback?code=auth_code_123&state="+url.QueryEscape(state), nil)
	rr := httptest.NewRecorder()

	h.ScaleKitCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	u, err := url.Parse(rr.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if u.Path != "/settings" {
		t.Fatalf("expected return_to redirect, got %s", u.Path)
	}
}

func TestScaleKitLogoutClearsSessionCookie(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/auth/logout?return_to=%2Flogin", nil)
	rr := httptest.NewRecorder()

	h.ScaleKitLogout(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/login" {
		t.Fatalf("unexpected logout redirect: %s", got)
	}
	var cleared bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "htc_token" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("expected htc_token cookie to be cleared")
	}
}

func TestSingleTenantLoginIssuesLocalSession(t *testing.T) {
	st := store.NewMemoryStore()
	h := &Handler{
		Store:                        st,
		AppPlan:                      "enterprise",
		AppDeploymentMode:            "single_tenant",
		PublicBaseURL:                "https://partner.example.com",
		MailDomain:                   "mail.partner.example.com",
		SingleTenantOwnerEmail:       "ops@partner.example.com",
		SingleTenantOwnerAlias:       "partner-ops",
		SingleTenantSetupTokenSHA256: security.HashValue("setup-secret"),
		AllowPublicRegistration:      false,
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/single-tenant/login", strings.NewReader(`{"setup_token":"setup-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.SingleTenantLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var token string
	for _, c := range rr.Result().Cookies() {
		if c.Name == "htc_token" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatalf("expected htc_token cookie")
	}
	acct, err := st.GetAccountByToken(context.Background(), token)
	if err != nil {
		t.Fatalf("token should resolve account: %v", err)
	}
	if acct.OwnerEmail != "ops@partner.example.com" {
		t.Fatalf("unexpected owner email: %s", acct.OwnerEmail)
	}
	if acct.PublicAlias != "partner-ops" {
		t.Fatalf("unexpected alias: %s", acct.PublicAlias)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	profile := body["app_profile"].(map[string]interface{})
	if profile["auth_mode"] != "single_tenant_setup_token" {
		t.Fatalf("unexpected auth mode: %v", profile["auth_mode"])
	}
	if profile["public_base_url"] != "https://partner.example.com" {
		t.Fatalf("unexpected public_base_url: %v", profile["public_base_url"])
	}
}

func TestSingleTenantLoginRejectsInvalidSetupToken(t *testing.T) {
	h := &Handler{
		Store:                        store.NewMemoryStore(),
		AppDeploymentMode:            "single_tenant",
		SingleTenantOwnerEmail:       "ops@partner.example.com",
		SingleTenantSetupTokenSHA256: security.HashValue("setup-secret"),
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/single-tenant/login", strings.NewReader(`{"setup_token":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.SingleTenantLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

func TestSingleTenantSetupTokenLoginDisabledWhenHashMissing(t *testing.T) {
	h := &Handler{
		Store:                  store.NewMemoryStore(),
		AppDeploymentMode:      "single_tenant",
		SingleTenantOwnerEmail: "ops@partner.example.com",
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/single-tenant/login", strings.NewReader(`{"setup_token":"anything"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.SingleTenantLogin(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestSingleTenantClaimIssuesLocalSession(t *testing.T) {
	st := store.NewMemoryStore()
	_, claimCode, created, err := st.EnsureSingleTenantClaim(context.Background(), "ops@partner.example.com", 24*time.Hour)
	if err != nil {
		t.Fatalf("ensure claim: %v", err)
	}
	if !created || claimCode == "" {
		t.Fatalf("expected new claim code")
	}
	h := &Handler{
		Store:                   st,
		AppPlan:                 "enterprise",
		AppDeploymentMode:       "single_tenant",
		PublicBaseURL:           "https://partner.example.com",
		MailDomain:              "mail.partner.example.com",
		SingleTenantOwnerEmail:  "ops@partner.example.com",
		AllowPublicRegistration: false,
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(`{"claim_code":"`+claimCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var token string
	for _, c := range rr.Result().Cookies() {
		if c.Name == "htc_token" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatalf("expected htc_token cookie")
	}
	acct, err := st.GetAccountByToken(context.Background(), token)
	if err != nil {
		t.Fatalf("token should resolve account: %v", err)
	}
	if acct.OwnerEmail != "ops@partner.example.com" {
		t.Fatalf("unexpected owner email: %s", acct.OwnerEmail)
	}
	if acct.PublicAlias != "ops" {
		t.Fatalf("expected alias derived from email local part, got %q", acct.PublicAlias)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	profile := body["app_profile"].(map[string]interface{})
	if profile["auth_mode"] != "single_tenant_claim" {
		t.Fatalf("unexpected auth mode: %v", profile["auth_mode"])
	}
}

func TestSingleTenantClaimRejectsReuseInvalidExpiredAndWrongOwner(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	_, claimCode, _, err := st.EnsureSingleTenantClaim(ctx, "ops@partner.example.com", time.Hour)
	if err != nil {
		t.Fatalf("ensure claim: %v", err)
	}
	h := &Handler{Store: st, AppDeploymentMode: "single_tenant", SingleTenantOwnerEmail: "ops@partner.example.com"}
	body := `{"claim_code":"` + claimCode + `"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first claim to pass, got %d: %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused claim 401, got %d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(`{"claim_code":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid claim 401, got %d", rr.Code)
	}
	_, expiredCode, _, err := st.EnsureSingleTenantClaim(ctx, "expired@partner.example.com", -time.Hour)
	if err != nil {
		t.Fatalf("ensure expired claim: %v", err)
	}
	h.SingleTenantOwnerEmail = "expired@partner.example.com"
	req = httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(`{"claim_code":"`+expiredCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired claim 401, got %d", rr.Code)
	}
	_, wrongOwnerCode, _, err := st.EnsureSingleTenantClaim(ctx, "first@partner.example.com", time.Hour)
	if err != nil {
		t.Fatalf("ensure wrong-owner claim: %v", err)
	}
	h.SingleTenantOwnerEmail = "second@partner.example.com"
	req = httptest.NewRequest(http.MethodPost, "/auth/single-tenant/claim", strings.NewReader(`{"claim_code":"`+wrongOwnerCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.SingleTenantClaim(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong-owner claim 401, got %d", rr.Code)
	}
}

func TestRegisterEmailBlockedInSingleTenantByDefault(t *testing.T) {
	h := &Handler{Store: store.NewMemoryStore(), AppDeploymentMode: "single_tenant"}
	req := httptest.NewRequest(http.MethodPost, "/api/register/email", strings.NewReader(`{"email":"ops@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.RegisterEmail(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rr.Code)
	}
}

func TestRegisterEmailStillWorksInMultitenant(t *testing.T) {
	h := &Handler{Store: store.NewMemoryStore()}
	req := httptest.NewRequest(http.MethodPost, "/api/register/email", strings.NewReader(`{"email":"ops@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.RegisterEmail(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEndToEndLocalFlow(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{Store: st, Pinecone: integrations.NewPineconeClient(false, "", "", "default"), LLM: integrations.NewLLMClient("", "", "", ""), Executor: service.NewActionService(nil)}
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

func TestCreateAndListForwardTargetsWithMetadata(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{Store: st, Pinecone: integrations.NewPineconeClient(false, "", "", "default"), LLM: integrations.NewLLMClient("", "", "", ""), Executor: service.NewActionService(nil)}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	regBody := []byte(`{"email":"7204909316@agentmail.to"}`)
	resp, err := http.Post(ts.URL+"/api/register/email", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reg map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	token := reg["token"].(string)

	targetReq := []byte(`{"target_key":"hubspot_primary","target_type":"http","purpose":"Primary CRM","enabled":true,"allowed_actions":["crm_upsert"],"auth":{"type":"bearer_header","secret_ref":"hubspot_api_key","header_name":"Authorization","prefix":"Bearer "},"config":{"url":"https://example.com/hubspot"}}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/forward-targets", bytes.NewReader(targetReq))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create forward target status %d", resp2.StatusCode)
	}
	var created map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&created)
	if created["target_key"] != "hubspot_primary" {
		t.Fatalf("expected target_key hubspot_primary, got %v", created["target_key"])
	}
	configJSON, _ := created["config_json"].(string)
	if !strings.Contains(configJSON, `"secret_ref":"hubspot_api_key"`) {
		t.Fatalf("expected auth secret ref in config json, got %s", configJSON)
	}

	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/forward-targets", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list forward targets status %d", resp3.StatusCode)
	}
	var targets []map[string]interface{}
	_ = json.NewDecoder(resp3.Body).Decode(&targets)
	if len(targets) != 1 {
		t.Fatalf("expected 1 forward target, got %d", len(targets))
	}
	if targets[0]["purpose"] != "Primary CRM" {
		t.Fatalf("expected purpose to round-trip, got %v", targets[0]["purpose"])
	}

	targetID, _ := created["id"].(string)
	updateReq := []byte(`{"target_key":"openclaw_primary","target_type":"http","purpose":"OpenClaw intake","enabled":true,"allowed_actions":["forward_http","crm_upsert"],"config":{"url":"https://example.com/openclaw","headers":{"x-source":"agenthook"}}}`)
	req3, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/forward-targets/"+targetID, bytes.NewReader(updateReq))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("update forward target status %d", resp4.StatusCode)
	}
	var updated map[string]interface{}
	_ = json.NewDecoder(resp4.Body).Decode(&updated)
	if updated["target_key"] != "openclaw_primary" {
		t.Fatalf("expected updated target key, got %v", updated["target_key"])
	}

	req4, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/forward-targets/"+targetID, nil)
	req4.Header.Set("Authorization", "Bearer "+token)
	resp5, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("delete forward target status %d", resp5.StatusCode)
	}

	req5, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/forward-targets", nil)
	req5.Header.Set("Authorization", "Bearer "+token)
	resp6, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatal(err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusOK {
		t.Fatalf("list forward targets after delete status %d", resp6.StatusCode)
	}
	var afterDelete []map[string]interface{}
	_ = json.NewDecoder(resp6.Body).Decode(&afterDelete)
	if len(afterDelete) != 0 {
		t.Fatalf("expected 0 forward targets after delete, got %d", len(afterDelete))
	}
}

func TestCreateAndRotateIntegrationSecrets(t *testing.T) {
	st := store.NewMemoryStore()
	proc := &service.Processor{Store: st, Pinecone: integrations.NewPineconeClient(false, "", "", "default"), LLM: integrations.NewLLMClient("", "", "", ""), Executor: service.NewActionService(nil)}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	regBody := []byte(`{"email":"7204909316@agentmail.to"}`)
	resp, err := http.Post(ts.URL+"/api/register/email", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reg map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	token := reg["token"].(string)

	createReq := []byte(`{"secret_key":"openclaw_api_key","purpose":"OpenClaw token","secret_value":"super-secret-token"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/integration-secrets", bytes.NewReader(createReq))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create integration secret status %d", resp2.StatusCode)
	}
	var created map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&created)
	if created["secret_value"] != nil {
		t.Fatalf("secret value should never be returned")
	}
	secretID, _ := created["id"].(string)

	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/integration-secrets", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list integration secrets status %d", resp3.StatusCode)
	}
	var secrets []map[string]interface{}
	_ = json.NewDecoder(resp3.Body).Decode(&secrets)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	if secrets[0]["secret_key"] != "openclaw_api_key" {
		t.Fatalf("expected secret key to round-trip, got %v", secrets[0]["secret_key"])
	}

	updateReq := []byte(`{"secret_key":"openclaw_api_key","purpose":"Rotated","secret_value":"rotated-secret-token"}`)
	req3, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/integration-secrets/"+secretID, bytes.NewReader(updateReq))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("update integration secret status %d", resp4.StatusCode)
	}

	resolved, err := st.ResolveIntegrationSecretValue(context.Background(), secrets[0]["account_id"].(string), "openclaw_api_key")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "rotated-secret-token" {
		t.Fatalf("expected rotated secret value to persist, got %q", resolved)
	}

	req4, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/integration-secrets/"+secretID, nil)
	req4.Header.Set("Authorization", "Bearer "+token)
	resp5, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("delete integration secret status %d", resp5.StatusCode)
	}
}
