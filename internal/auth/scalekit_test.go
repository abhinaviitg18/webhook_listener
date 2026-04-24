package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"agenthook.store/internal/store"
)

func TestTokenVerifier_LocalToken(t *testing.T) {
	st := store.NewMemoryStore()
	acct, token, err := st.CreateAccount(context.Background(), "techhiring@agentmail.to")
	if err != nil {
		t.Fatal(err)
	}
	v := TokenVerifier{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	got, err := v.VerifyRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != acct.ID {
		t.Fatalf("expected account %s got %s", acct.ID, got.ID)
	}
}

func TestTokenVerifier_LocalTokenFromCookie(t *testing.T) {
	st := store.NewMemoryStore()
	acct, token, err := st.CreateAccount(context.Background(), "techhiring@agentmail.to")
	if err != nil {
		t.Fatal(err)
	}
	v := TokenVerifier{Store: st}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "htc_token", Value: token})
	got, err := v.VerifyRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != acct.ID {
		t.Fatalf("expected account %s got %s", acct.ID, got.ID)
	}
}

func TestTokenVerifier_ScaleKitFallbackProvisioning(t *testing.T) {
	st := store.NewMemoryStore()
	var bearerSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			base := fmt.Sprintf("%s://%s", scheme, r.Host)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"userinfo_endpoint": base + "/oauth2/userinfo",
			})
		case "/oauth2/userinfo":
			bearerSeen = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"email": "7204909316@agentmail.to",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	v := TokenVerifier{
		Store:           st,
		ScaleKitBaseURL: srv.URL,
		ScaleKitAPIKey:  "test-key",
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer scalekit_token_1")
	got, err := v.VerifyRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerEmail != "7204909316@agentmail.to" {
		t.Fatalf("unexpected owner email: %s", got.OwnerEmail)
	}
	if bearerSeen != "Bearer scalekit_token_1" {
		t.Fatalf("expected bearer token forwarded to userinfo, got %q", bearerSeen)
	}
}
