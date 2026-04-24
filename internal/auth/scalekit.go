package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agenthook.store/internal/domain"
)

type RequestVerifier interface {
	VerifyRequest(r *http.Request) (domain.Account, error)
}

type TokenVerifier struct {
	Store           domain.Store
	ScaleKitBaseURL string
	ScaleKitAPIKey  string
	HTTPClient      *http.Client
}

func (v TokenVerifier) VerifyRequest(r *http.Request) (domain.Account, error) {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	token := ""
	if strings.HasPrefix(authz, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	}
	if token == "" {
		if cookie, err := r.Cookie("htc_token"); err == nil {
			token = strings.TrimSpace(cookie.Value)
		}
	}
	if token == "" {
		return domain.Account{}, errors.New("empty token")
	}

	// 1) Local bearer token auth.
	if acct, err := v.Store.GetAccountByToken(r.Context(), token); err == nil {
		return acct, nil
	}

	// 2) ScaleKit bearer token auth via OIDC userinfo.
	if strings.TrimSpace(v.ScaleKitBaseURL) == "" {
		return domain.Account{}, errors.New("unauthorized")
	}
	email, err := v.lookupScaleKitEmail(r.Context(), token)
	if err != nil {
		return domain.Account{}, errors.New("unauthorized")
	}
	slug := slugFromEmail(email)
	if slug == "" {
		return domain.Account{}, errors.New("unauthorized")
	}
	if acct, err := v.Store.GetAccountBySlug(r.Context(), slug); err == nil {
		return acct, nil
	}
	if _, _, err := v.Store.CreateAccount(r.Context(), email); err != nil {
		return domain.Account{}, err
	}
	return v.Store.GetAccountBySlug(r.Context(), slug)
}

type ctxKey string

const AccountCtxKey ctxKey = "account"

func AccountFromContext(ctx context.Context) (domain.Account, bool) {
	acct, ok := ctx.Value(AccountCtxKey).(domain.Account)
	return acct, ok
}

func (v TokenVerifier) lookupScaleKitEmail(ctx context.Context, bearerToken string) (string, error) {
	userInfoURL, err := v.resolveUserInfoURL(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	resp, err := v.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("scalekit userinfo status: %s", resp.Status)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	email := strings.TrimSpace(asString(out["email"]))
	if email == "" {
		if profile, ok := out["profile"].(map[string]interface{}); ok {
			email = strings.TrimSpace(asString(profile["email"]))
		}
	}
	if email == "" {
		return "", errors.New("email missing in scalekit token")
	}
	return strings.ToLower(email), nil
}

func (v TokenVerifier) resolveUserInfoURL(ctx context.Context) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(v.ScaleKitBaseURL), "/")
	if base == "" {
		return "", errors.New("scalekit base url missing")
	}
	wellKnownURL := base + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return "", err
	}
	if key := strings.TrimSpace(v.ScaleKitAPIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := v.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("scalekit discovery status: %s", resp.Status)
	}
	var cfg struct {
		UserInfoEndpoint string `json:"userinfo_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.UserInfoEndpoint) == "" {
		return "", errors.New("userinfo_endpoint missing")
	}
	return strings.TrimSpace(cfg.UserInfoEndpoint), nil
}

func (v TokenVerifier) httpClient() *http.Client {
	if v.HTTPClient != nil {
		return v.HTTPClient
	}
	return &http.Client{Timeout: 6 * time.Second}
}

func slugFromEmail(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ""
	}
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
