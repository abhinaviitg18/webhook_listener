package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/store"
)

func TestActionServiceUsesNamedIntegrationSecretForBearerAuth(t *testing.T) {
	st := store.NewMemoryStore()
	account := domain.Account{ID: "acct-1"}
	if _, err := st.CreateIntegrationSecret(context.Background(), domain.IntegrationSecret{
		AccountID:   account.ID,
		SecretKey:   "openclaw_api_key",
		Purpose:     "OpenClaw bearer",
		SecretValue: "named-secret-token",
	}); err != nil {
		t.Fatal(err)
	}

	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	target := domain.ForwardTarget{
		ID:         "target-1",
		AccountID:  account.ID,
		TargetType: "http",
		ConfigJSON: BuildIntegrationTargetConfig("openclaw_primary", "OpenClaw", true, []string{"forward_http"}, nil, map[string]interface{}{"url": srv.URL}, IntegrationTargetAuthConfig{
			Type:       "bearer_header",
			SecretRef:  "openclaw_api_key",
			HeaderName: "Authorization",
			Prefix:     "Bearer ",
		}, nil, nil),
	}

	svc := NewActionService(nil)
	svc.Store = st
	svc.LookupEnv = func(string) (string, bool) { return "", false }

	err := svc.Execute(context.Background(), domain.ProcessDecision{
		ActionName: "forward_http",
		Params: map[string]interface{}{
			"integration_target_key": "openclaw_primary",
		},
	}, account, domain.WebhookEvent{
		ID:          "evt-1",
		AccountID:   account.ID,
		TypeKey:     "lis::generic-json::openclaw::multitenant",
		PayloadJSON: `{"ok":true}`,
	}, []domain.ForwardTarget{target})
	if err != nil {
		t.Fatal(err)
	}
	if seenAuth != "Bearer named-secret-token" {
		t.Fatalf("expected named secret bearer auth, got %q", seenAuth)
	}
}

func TestActionServiceUsesSingleTenantEnvFallback(t *testing.T) {
	account := domain.Account{ID: "acct-2"}
	var seenHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	target := domain.ForwardTarget{
		ID:         "target-2",
		AccountID:  account.ID,
		TargetType: "http",
		ConfigJSON: BuildIntegrationTargetConfig("openclaw_primary", "OpenClaw", true, []string{"forward_http"}, nil, map[string]interface{}{"url": srv.URL}, IntegrationTargetAuthConfig{
			Type:       "custom_header",
			SecretRef:  "",
			HeaderName: "X-Api-Key",
		}, nil, nil),
	}

	svc := NewActionService(nil)
	svc.LookupEnv = func(key string) (string, bool) {
		if key == "OPENCLAW_API_KEY" {
			return "single-tenant-env-token", true
		}
		return "", false
	}

	err := svc.Execute(context.Background(), domain.ProcessDecision{
		ActionName: "forward_http",
		Params: map[string]interface{}{
			"integration_target_key": "openclaw_primary",
		},
	}, account, domain.WebhookEvent{
		ID:          "evt-2",
		AccountID:   account.ID,
		TypeKey:     "lis::generic-json::openclaw::single_tenant",
		PayloadJSON: `{"ok":true}`,
	}, []domain.ForwardTarget{target})
	if err != nil {
		t.Fatal(err)
	}
	if seenHeader != "single-tenant-env-token" {
		t.Fatalf("expected single-tenant env fallback header, got %q", seenHeader)
	}
}

func TestActionServiceUsesExplicitHeaderSecretRefAndQueryAuth(t *testing.T) {
	st := store.NewMemoryStore()
	account := domain.Account{ID: "acct-3"}
	if _, err := st.CreateIntegrationSecret(context.Background(), domain.IntegrationSecret{
		AccountID:   account.ID,
		SecretKey:   "crm_bearer_token",
		Purpose:     "CRM key",
		SecretValue: "crm-secret",
	}); err != nil {
		t.Fatal(err)
	}

	var seenQuery string
	var seenHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.Query().Get("api_key")
		seenHeader = r.Header.Get("X-Crm-Key")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	target := domain.ForwardTarget{
		ID:         "target-3",
		AccountID:  account.ID,
		TargetType: "http",
		ConfigJSON: BuildIntegrationTargetConfig("crm_primary", "CRM", true, []string{"crm_upsert"}, nil, map[string]interface{}{"url": srv.URL}, IntegrationTargetAuthConfig{
			Type:       "query_param",
			SecretRef:  "crm_bearer_token",
			QueryParam: "api_key",
		}, map[string]string{"X-Crm-Key": "crm_bearer_token"}, nil),
	}

	svc := NewActionService(nil)
	svc.Store = st
	err := svc.Execute(context.Background(), domain.ProcessDecision{
		ActionName: "crm_upsert",
		Params: map[string]interface{}{
			"integration_target_key": "crm_primary",
			"entity_payload":         map[string]interface{}{"name": "Acme"},
		},
	}, account, domain.WebhookEvent{
		ID:          "evt-3",
		AccountID:   account.ID,
		TypeKey:     "lis::generic-json::crm::multitenant",
		PayloadJSON: mustJSONMap(t, map[string]interface{}{"ok": true}),
	}, []domain.ForwardTarget{target})
	if err != nil {
		t.Fatal(err)
	}
	if seenQuery != "crm-secret" {
		t.Fatalf("expected query auth token, got %q", seenQuery)
	}
	if seenHeader != "crm-secret" {
		t.Fatalf("expected header secret ref token, got %q", seenHeader)
	}
}

func mustJSONMap(t *testing.T, value map[string]interface{}) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
