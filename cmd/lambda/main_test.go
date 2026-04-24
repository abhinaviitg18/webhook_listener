package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestApplyEnvFileSkipsExistingValues(t *testing.T) {
	t.Setenv("EXISTING_KEY", "keep-me")

	if err := applyEnvFile("EXISTING_KEY=replace-me\nNEW_KEY=created\n"); err != nil {
		t.Fatalf("applyEnvFile returned error: %v", err)
	}

	if got := os.Getenv("EXISTING_KEY"); got != "keep-me" {
		t.Fatalf("expected EXISTING_KEY to stay unchanged, got %q", got)
	}
	if got := os.Getenv("NEW_KEY"); got != "created" {
		t.Fatalf("expected NEW_KEY to be created, got %q", got)
	}
}

func TestToAPIGatewayV2RequestPreservesFunctionURLFields(t *testing.T) {
	req := events.LambdaFunctionURLRequest{
		Version:        "2.0",
		RawPath:        "/auth/scalekit/callback",
		RawQueryString: "code=abc",
		Cookies:        []string{"a=b"},
		Headers:        map[string]string{"host": "app.agenthook.store"},
		RequestContext: events.LambdaFunctionURLRequestContext{
			AccountID:  "123",
			RequestID:  "req-1",
			APIID:      "api-1",
			DomainName: "example.lambda-url.us-east-1.on.aws",
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method:   http.MethodGet,
				Path:     "/auth/scalekit/callback",
				SourceIP: "127.0.0.1",
			},
		},
	}

	got := toAPIGatewayV2Request(req)
	if got.RawPath != req.RawPath {
		t.Fatalf("expected raw path %q, got %q", req.RawPath, got.RawPath)
	}
	if got.RequestContext.HTTP.Method != http.MethodGet {
		t.Fatalf("expected method %q, got %q", http.MethodGet, got.RequestContext.HTTP.Method)
	}
	if got.RequestContext.DomainName != req.RequestContext.DomainName {
		t.Fatalf("expected domain name %q, got %q", req.RequestContext.DomainName, got.RequestContext.DomainName)
	}
}

func TestWithOriginSecretRejectsUnexpectedSecret(t *testing.T) {
	t.Setenv(lambdaOriginSecretEnv, "expected-secret")

	handler := withOriginSecret(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "https://app.agenthook.store/", nil)
	req.Header.Set(originSecretHeader, "wrong-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", rec.Code)
	}
}
