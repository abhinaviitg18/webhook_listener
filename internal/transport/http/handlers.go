package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/service"
	"agenthook.store/internal/ui"
)

type Handler struct {
	Store                domain.Store
	Processor            *service.Processor
	VerifyHTCSignature   bool
	ScaleKitBaseURL      string
	ScaleKitClientID     string
	ScaleKitClientSecret string
	ScaleKitRedirectURI  string
	AppSessionSecret     string
	PublicBaseURL        string
}

func NewRouter(h *Handler, verifier auth.RequestVerifier) http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	r.Post("/api/register/email", h.RegisterEmail)
	r.Get("/auth/scalekit/start", h.ScaleKitStart)
	r.Get("/auth/scalekit/login", h.ScaleKitLoginRedirect)
	r.Get("/auth/scalekit/signup", h.ScaleKitSignupRedirect)
	r.Get("/auth/scalekit/callback", h.ScaleKitCallback)
	r.Get("/auth/logout", h.ScaleKitLogout)

	r.Group(func(ar chi.Router) {
		ar.Use(AuthMiddleware(verifier))
		ar.Post("/api/webhooks/types", h.CreateType)
		ar.Get("/api/webhooks/types", h.ListTypes)
		ar.Post("/api/webhooks/secrets", h.CreateSecret)
		ar.Delete("/api/webhooks/secrets/{secretID}", h.DeleteSecret)
		ar.Post("/api/forward-targets", h.CreateForwardTarget)
		ar.Get("/api/events", h.ListEvents)
		ar.Post("/api/events/{eventID}/re-run", h.ReprocessEvent)
		ar.Get("/api/events/{eventID}", h.GetEvent)
		ar.Post("/api/resolver/signatures", h.CreateSignature)
		ar.Get("/api/resolver/signatures", h.ListSignatures)
		ar.Post("/api/resolver/transforms", h.CreateTransform)
		ar.Get("/api/resolver/transforms", h.ListTransforms)
		ar.Post("/api/resolver/classify", h.DryRunClassify)
		ar.Post("/api/resolver/transform", h.DryRunTransform)
		ar.Post("/api/policy/master", h.UpsertMasterPromptPolicy)
		ar.Get("/api/policy/master", h.GetMasterPromptPolicy)
		ar.Post("/api/policy/skills", h.CreateWebhookSkill)
		ar.Get("/api/policy/skills", h.ListWebhookSkills)
		ar.Post("/api/policy/skills/dry-run", h.DryRunSkills)
		ar.Get("/api/me", h.UserInfo)

		ar.Post("/v1/listeners", h.CreateListener)
		ar.Get("/v1/listeners", h.ListListeners)
		ar.Delete("/v1/listeners/{listenerID}", h.DeleteListener)
		ar.Post("/v1/listeners/{listenerID}/secrets", h.CreateListenerSecret)
		ar.Get("/v1/listeners/{listenerID}/secrets", h.ListListenerSecrets)
		ar.Get("/v1/listeners/{listenerID}/events", h.ListListenerEvents)
		ar.Post("/v1/auth/tokens", h.CreateAPIToken)
		ar.Post("/v1/presets/webhook-processing", h.ApplyWebhookProcessingPreset)
		ar.Post("/v1/byok/providers/test", h.TestBYOKProviders)
		ar.Post("/v1/byok/providers", h.UpsertBYOKProvider)
		ar.Get("/v1/byok/providers", h.ListBYOKProviders)
	})

	r.Post("/url/{account}/{type}/{secret}", h.ReceiveWebhook)
	r.Post("/url/{account}/{secret}", h.ReceiveWebhookAuto)
	r.Post("/ingest/{account}/{provider}/{webhookID}/{secret}", h.ReceiveWebhookByProvider)

	// Serve the embedded static frontend for all other routes (for SPA support)
	r.Handle("/*", ui.StaticHandler())

	return r
}

func (h *Handler) RegisterEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || !strings.Contains(body.Email, "@") {
		writeErr(w, http.StatusBadRequest, "valid email required")
		return
	}
	acct, token, err := h.Store.CreateAccount(r.Context(), body.Email)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"account": acct,
		"token":   token,
		"note":    "send this token by email in production",
	})
}

func (h *Handler) ScaleKitStart(w http.ResponseWriter, r *http.Request) {
	if h.hasScaleKitOAuthConfig() {
		authURL, err := h.scaleKitAuthorizationURL(r)
		if err == nil {
			http.Redirect(w, r, authURL, http.StatusFound)
			return
		}
	}

	intent := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("intent")))
	if intent == "signup" {
		h.redirectToHostedScaleKitPage(w, r, "/a/auth/signup")
		return
	}
	h.redirectToHostedScaleKitPage(w, r, "/a/auth/login")
}

func (h *Handler) ScaleKitLoginRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("intent", "signin")
	r.URL.RawQuery = q.Encode()
	h.ScaleKitStart(w, r)
}

func (h *Handler) ScaleKitSignupRedirect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	q.Set("intent", "signup")
	r.URL.RawQuery = q.Encode()
	h.ScaleKitStart(w, r)
}

func (h *Handler) ScaleKitCallback(w http.ResponseWriter, r *http.Request) {
	if errCode := strings.TrimSpace(r.URL.Query().Get("error")); errCode != "" {
		target := h.appRedirectURL(r, "")
		q := target.Query()
		q.Set("auth_error", errCode)
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	// 1. Get code from query
	code := r.URL.Query().Get("code")
	if code == "" {
		// If no code, maybe it's an error or we're already authenticated
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	returnTo := ""
	if h.hasScaleKitOAuthConfig() {
		parsedReturnTo, err := h.parseScaleKitState(r.URL.Query().Get("state"))
		if err != nil {
			target := h.appRedirectURL(r, "")
			q := target.Query()
			q.Set("auth_error", "invalid_state")
			target.RawQuery = q.Encode()
			http.Redirect(w, r, target.String(), http.StatusFound)
			return
		}
		returnTo = parsedReturnTo
	}
	target := h.appRedirectURL(r, returnTo)
	q := target.Query()

	// 2. Exchange ScaleKit auth code server-side and mint a real app token.
	if localToken, err := h.exchangeScaleKitCodeToLocalToken(r.Context(), code); err == nil && localToken != "" {
		h.writeSessionCookie(w, localToken, 3600*24*30)
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	// 3. If ScaleKit is configured but exchange failed, avoid recycling the raw code
	// (which can trigger a callback/login loop on the frontend).
	if h.isScaleKitConfigured() {
		q.Set("auth_error", "scalekit_exchange_failed")
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	// 4. Compatibility fallback for local/dev tests without ScaleKit config.
	q.Set("code", code)
	target.RawQuery = q.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (h *Handler) scalekitCallbackURL(r *http.Request) string {
	if v := strings.TrimSpace(h.ScaleKitRedirectURI); v != "" {
		return v
	}
	if base := strings.TrimRight(strings.TrimSpace(h.PublicBaseURL), "/"); base != "" {
		return base + "/auth/scalekit/callback"
	}
	return "https://app.agenthook.store/auth/scalekit/callback"
}

func (h *Handler) UserInfo(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (h *Handler) scalekitBase() string {
	base := strings.TrimSpace(h.ScaleKitBaseURL)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("SCALEKIT_BASE_URL"))
	}
	if base == "" {
		base = "https://hookweb.scalekit.com"
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		// Ensure consistently using .com vs .dev if specified by ScaleKit best practices
		if strings.HasSuffix(parsed.Host, ".scalekit.dev") {
			parsed.Host = strings.TrimSuffix(parsed.Host, ".dev") + ".com"
			return strings.TrimRight(parsed.String(), "/")
		}
		return strings.TrimRight(base, "/")
	}
	return base
}

func (h *Handler) isScaleKitConfigured() bool {
	return strings.TrimSpace(h.ScaleKitBaseURL) != "" || strings.TrimSpace(os.Getenv("SCALEKIT_BASE_URL")) != ""
}

func (h *Handler) hasScaleKitOAuthConfig() bool {
	return strings.TrimSpace(h.ScaleKitClientID) != "" && strings.TrimSpace(h.ScaleKitClientSecret) != "" && h.isScaleKitConfigured()
}

func (h *Handler) ScaleKitLogout(w http.ResponseWriter, r *http.Request) {
	h.writeSessionCookie(w, "", -1)
	target := "/"
	if r.URL.Query().Get("return_to") != "" {
		target = r.URL.Query().Get("return_to")
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) writeSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "htc_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (h *Handler) redirectToHostedScaleKitPage(w http.ResponseWriter, r *http.Request, path string) {
	base := h.scalekitBase()
	redirectURI := h.scalekitCallbackURL(r)
	u, _ := url.Parse(strings.TrimRight(base, "/") + path)
	q := u.Query()
	q.Set("redirect_uri", redirectURI)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (h *Handler) scaleKitAuthorizationURL(r *http.Request) (string, error) {
	state, err := h.mintScaleKitState(r.URL.Query().Get("return_to"))
	if err != nil {
		return "", err
	}
	u, err := url.Parse(strings.TrimRight(h.scalekitBase(), "/") + "/oauth/authorize")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", strings.TrimSpace(h.ScaleKitClientID))
	q.Set("redirect_uri", h.scalekitCallbackURL(r))
	q.Set("scope", "openid profile email")
	q.Set("state", state)
	if loginHint := strings.TrimSpace(r.URL.Query().Get("login_hint")); loginHint != "" {
		q.Set("login_hint", loginHint)
	}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("intent")), "signup") {
		q.Set("screen_hint", "signup")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type scaleKitStatePayload struct {
	ReturnTo string `json:"return_to"`
	ExpUnix  int64  `json:"exp_unix"`
}

func (h *Handler) mintScaleKitState(returnTo string) (string, error) {
	secret := strings.TrimSpace(h.AppSessionSecret)
	if secret == "" {
		return "", errors.New("APP_SESSION_SECRET is required")
	}
	payload := scaleKitStatePayload{
		ReturnTo: h.sanitizeReturnTo(returnTo),
		ExpUnix:  time.Now().Add(15 * time.Minute).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

func (h *Handler) parseScaleKitState(state string) (string, error) {
	secret := strings.TrimSpace(h.AppSessionSecret)
	if secret == "" {
		return "", errors.New("APP_SESSION_SECRET is required")
	}
	parts := strings.Split(strings.TrimSpace(state), ".")
	if len(parts) != 2 {
		return "", errors.New("invalid auth state")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", errors.New("invalid auth state signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var payload scaleKitStatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if payload.ExpUnix <= time.Now().Unix() {
		return "", errors.New("auth state expired")
	}
	return h.sanitizeReturnTo(payload.ReturnTo), nil
}

func (h *Handler) sanitizeReturnTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/app"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "/app"
	}
	if u.IsAbs() {
		base, err := url.Parse(strings.TrimSpace(h.publicBaseURL()))
		if err != nil || base.Hostname() == "" || !strings.EqualFold(base.Hostname(), u.Hostname()) {
			return "/app"
		}
		return u.RequestURI()
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "/app"
	}
	return u.RequestURI()
}

func (h *Handler) appRedirectURL(r *http.Request, returnTo string) url.URL {
	base, err := url.Parse(h.publicBaseURL())
	if err != nil || base.Scheme == "" || base.Host == "" {
		base = &url.URL{Scheme: "https", Host: "app.agenthook.store"}
	}
	if path := strings.TrimSpace(returnTo); path != "" {
		base.Path = path
		base.RawQuery = ""
		return *base
	}
	base.Path = "/app"
	base.RawQuery = ""
	return *base
}

func (h *Handler) publicBaseURL() string {
	if v := strings.TrimRight(strings.TrimSpace(h.PublicBaseURL), "/"); v != "" {
		return v
	}
	return "https://app.agenthook.store"
}

func (h *Handler) exchangeScaleKitCodeToLocalToken(ctx context.Context, code string) (string, error) {
	if !h.isScaleKitConfigured() {
		return "", errors.New("scalekit not configured")
	}

	base := strings.TrimRight(h.scalekitBase(), "/")
	redirectURI := "https://app.agenthook.store/auth/scalekit/callback"
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		},
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(h.ScaleKitClientID) != "" {
		form.Set("client_id", strings.TrimSpace(h.ScaleKitClientID))
	}
	if strings.TrimSpace(h.ScaleKitClientSecret) != "" {
		form.Set("client_secret", strings.TrimSpace(h.ScaleKitClientSecret))
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", err
	}
	defer tokenResp.Body.Close()

	var tokenOut struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenOut); err != nil {
		return "", err
	}
	if tokenResp.StatusCode >= 300 || strings.TrimSpace(tokenOut.AccessToken) == "" {
		return "", errors.New("failed to exchange scalekit code")
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/userinfo", nil)
	if err != nil {
		return "", err
	}
	userReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tokenOut.AccessToken))

	userResp, err := client.Do(userReq)
	if err != nil {
		return "", err
	}
	defer userResp.Body.Close()
	if userResp.StatusCode >= 300 {
		return "", errors.New("failed to fetch scalekit userinfo")
	}

	var userOut map[string]interface{}
	if err := json.NewDecoder(userResp.Body).Decode(&userOut); err != nil {
		return "", err
	}
	email := strings.TrimSpace(strings.ToLower(asString(userOut["email"])))
	if email == "" {
		if profile, ok := userOut["profile"].(map[string]interface{}); ok {
			email = strings.TrimSpace(strings.ToLower(asString(profile["email"])))
		}
	}
	if email == "" {
		return "", errors.New("missing email in scalekit userinfo")
	}

	_, localToken, err := h.Store.CreateAccount(ctx, email)
	if err != nil {
		return "", err
	}
	return localToken, nil
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func (h *Handler) CreateType(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TypeKey         string `json:"type_key"`
		PlainTextAction string `json:"plain_text_action"`
		UseLLMFallback  bool   `json:"use_llm_fallback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.TypeKey = strings.TrimSpace(body.TypeKey)
	if body.TypeKey == "" {
		writeErr(w, http.StatusBadRequest, "type_key required")
		return
	}
	t, err := h.Store.CreateWebhookType(r.Context(), acct.ID, body.TypeKey, body.PlainTextAction, body.UseLLMFallback)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.Store.ListWebhookTypes(r.Context(), acct.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateSecret(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TypeKey string `json:"type_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	whType, err := h.Store.GetWebhookTypeByAccountAndKey(r.Context(), acct.ID, strings.TrimSpace(body.TypeKey))
	if err != nil {
		writeErr(w, http.StatusNotFound, "type not found")
		return
	}
	secret, raw, err := h.Store.CreateSecret(r.Context(), acct.ID, whType.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	url := "/url/" + acct.Slug + "/" + whType.TypeKey + "/" + raw
	writeJSON(w, http.StatusCreated, map[string]any{"secret": secret, "webhook_url": url, "secret_value": raw})
}

func (h *Handler) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "secretID")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "secretID required")
		return
	}
	if err := h.Store.DeleteSecret(r.Context(), acct.ID, id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) CreateForwardTarget(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TargetType string          `json:"target_type"`
		Config     json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	t, err := h.Store.CreateForwardTarget(r.Context(), acct.ID, strings.TrimSpace(body.TargetType), string(body.Config))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	events, err := h.Store.ListEvents(r.Context(), acct.ID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if strings.TrimSpace(acct.OwnerEmail) == "" {
		writeErr(w, http.StatusBadRequest, "account missing owner email")
		return
	}
	_, token, err := h.Store.CreateAccount(r.Context(), acct.OwnerEmail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   token,
		"account": acct.Slug,
		"note":    "new API token created",
	})
}

func (h *Handler) ApplyWebhookProcessingPreset(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Provider        string `json:"provider"`
		ListenerID      string `json:"listener_id"`
		GeneralPrompt   string `json:"general_prompt"`
		SpecificPrompt  string `json:"specific_prompt"`
		SpecificMatch   string `json:"specific_match_contains"`
		SpecificAction  string `json:"specific_action"`
		MemoryWriteMode string `json:"memory_write_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	provider := normalizeProvider(body.Provider)
	listenerID := normalizeListenerID(body.ListenerID)
	if provider == "" || listenerID == "" {
		writeErr(w, http.StatusBadRequest, "provider and listener_id are required")
		return
	}
	whType, err := h.findListenerType(r.Context(), h.Store, acct.ID, provider, listenerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	generalPrompt := strings.TrimSpace(body.GeneralPrompt)
	if generalPrompt == "" {
		generalPrompt = "Always preserve raw payload for audit. Prefer deterministic action; use LLM only when deterministic logic is unavailable."
	}
	specificPrompt := strings.TrimSpace(body.SpecificPrompt)
	if specificPrompt == "" {
		specificPrompt = "For this provider, extract key fields and store concise processed text for operators."
	}
	specificMatch := strings.TrimSpace(body.SpecificMatch)
	if specificMatch == "" {
		specificMatch = provider
	}
	specificAction := strings.TrimSpace(body.SpecificAction)
	if specificAction == "" {
		specificAction = "store_mysql"
	}
	mode := strings.TrimSpace(body.MemoryWriteMode)
	if mode == "" {
		mode = "update_or_insert"
	}
	policy, err := h.Store.UpsertMasterPromptPolicy(r.Context(), acct.ID, generalPrompt, acct.OwnerEmail)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	generalSkill, gErr := h.Store.CreateWebhookSkill(r.Context(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         whType.TypeKey,
		SkillKey:        "general-route-" + shortID(),
		SkillPrompt:     "General webhook handling baseline",
		MatchContains:   "",
		ForcedAction:    "store_mysql",
		MemoryWriteMode: "update_or_insert",
		Priority:        999,
		Enabled:         true,
	})
	specificSkill, sErr := h.Store.CreateWebhookSkill(r.Context(), domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         whType.TypeKey,
		SkillKey:        provider + "-specific-" + shortID(),
		SkillPrompt:     specificPrompt,
		MatchContains:   specificMatch,
		ForcedAction:    specificAction,
		MemoryWriteMode: mode,
		Priority:        100,
		Enabled:         true,
	})
	if gErr != nil || sErr != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Sprintf("preset apply failed (general=%v specific=%v)", gErr, sErr))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"policy":         policy,
		"general_skill":  generalSkill,
		"specific_skill": specificSkill,
		"type_key":       whType.TypeKey,
	})
}

func (h *Handler) TestBYOKProviders(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		GroqAPIKey      string `json:"groq_api_key"`
		GroqBaseURL     string `json:"groq_base_url"`
		CerebrasAPIKey  string `json:"cerebras_api_key"`
		CerebrasBaseURL string `json:"cerebras_base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	groqBase := strings.TrimSpace(body.GroqBaseURL)
	if groqBase == "" {
		groqBase = "https://api.groq.com/openai/v1"
	}
	cereBase := strings.TrimSpace(body.CerebrasBaseURL)
	if cereBase == "" {
		cereBase = "https://api.cerebras.ai/v1"
	}
	out := map[string]any{
		"byok": true,
		"providers": map[string]any{
			"groq":     providerTokenTest(r.Context(), groqBase, strings.TrimSpace(body.GroqAPIKey)),
			"cerebras": providerTokenTest(r.Context(), cereBase, strings.TrimSpace(body.CerebrasAPIKey)),
		},
		"note": "Token creation on provider side must be done in provider console; this endpoint validates supplied BYOK keys.",
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) UpsertBYOKProvider(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Provider  string `json:"provider"`
		APIKey    string `json:"api_key"`
		BaseURL   string `json:"base_url"`
		Model     string `json:"model"`
		IsDefault bool   `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	provider := strings.TrimSpace(strings.ToLower(body.Provider))
	if provider == "" || strings.TrimSpace(body.APIKey) == "" {
		writeErr(w, http.StatusBadRequest, "provider and api_key are required")
		return
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL == "" {
		switch provider {
		case "groq":
			baseURL = "https://api.groq.com/openai/v1"
		case "cerebras":
			baseURL = "https://api.cerebras.ai/v1"
		case "openai":
			baseURL = "https://api.openai.com/v1"
		default:
			baseURL = "https://openrouter.ai/api/v1"
		}
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		switch provider {
		case "groq":
			model = "llama3-70b-8192"
		case "cerebras":
			model = "llama3.1-70b"
		case "openai":
			model = "gpt-4o-mini"
		default:
			model = "meta-llama/llama-3-70b-instruct"
		}
	}
	cfg, err := h.Store.UpsertBYOKConfig(r.Context(), domain.BYOKProviderConfig{
		AccountID: acct.ID,
		Provider:  provider,
		APIKey:    strings.TrimSpace(body.APIKey),
		BaseURL:   baseURL,
		Model:     model,
		IsDefault: body.IsDefault,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, cfg)
}

func (h *Handler) ListBYOKProviders(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cfgs, err := h.Store.ListBYOKConfigs(r.Context(), acct.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfgs)
}

func (h *Handler) CreateListener(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Provider        string `json:"provider"`
		ListenerID      string `json:"listener_id"`
		DeploymentMode  string `json:"deployment_mode"`
		PlainTextAction string `json:"plain_text_action"`
		UseLLMFallback  bool   `json:"use_llm_fallback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	provider := normalizeProvider(body.Provider)
	if provider == "" {
		writeErr(w, http.StatusBadRequest, "provider is required")
		return
	}
	listenerID := normalizeListenerID(body.ListenerID)
	if listenerID == "" {
		listenerID = "wh_" + strings.ReplaceAll(uuid.NewString()[:12], "-", "")
	}
	deploymentMode := normalizeDeploymentMode(body.DeploymentMode)
	typeKey := buildListenerTypeKey(provider, listenerID, deploymentMode)
	whType, err := h.Store.CreateWebhookType(r.Context(), acct.ID, typeKey, strings.TrimSpace(body.PlainTextAction), body.UseLLMFallback)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	secret, raw, err := h.Store.CreateSecret(r.Context(), acct.ID, whType.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseUrl := h.publicBaseURL()
	writeJSON(w, http.StatusCreated, map[string]any{
		"listener_id":      listenerID,
		"provider":         provider,
		"deployment_mode":  deploymentMode,
		"type_key":         whType.TypeKey,
		"secret_id":        secret.ID,
		"secret_value":     raw,
		"webhook_url":      baseUrl + "/ingest/" + acct.Slug + "/" + provider + "/" + listenerID + "/" + raw,
		"legacy_webhook":   baseUrl + "/url/" + acct.Slug + "/" + whType.TypeKey + "/" + raw,
		"required_headers": []string{"Content-Type: application/json"},
	})
}

func (h *Handler) ListListeners(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.Store.ListWebhookTypes(r.Context(), acct.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ref, ok := parseListenerTypeKey(item.TypeKey)
		if !ok {
			continue
		}
		baseUrl := h.publicBaseURL()
		resp = append(resp, map[string]any{
			"listener_id":           ref.ListenerID,
			"provider":              ref.Provider,
			"deployment_mode":       ref.DeploymentMode,
			"type_key":              item.TypeKey,
			"plain_text_action":     item.PlainTextAction,
			"use_llm_fallback":      item.UseLLMFallback,
			"created_at":            item.CreatedAt,
			"webhook_url_template":  baseUrl + "/ingest/" + acct.Slug + "/" + ref.Provider + "/" + ref.ListenerID + "/[secret]",
			"legacy_webhook_url":    baseUrl + "/url/" + acct.Slug + "/" + item.TypeKey + "/[secret]",
			"listener_display_name": ref.Provider + " · " + ref.ListenerID,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateListenerSecret(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	listenerID := normalizeListenerID(chi.URLParam(r, "listenerID"))
	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	provider := normalizeProvider(body.Provider)
	if provider == "" || listenerID == "" {
		writeErr(w, http.StatusBadRequest, "provider and listenerID required")
		return
	}
	whType, err := h.findListenerType(r.Context(), h.Store, acct.ID, provider, listenerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	secret, raw, err := h.Store.CreateSecret(r.Context(), acct.ID, whType.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseUrl := h.publicBaseURL()
	writeJSON(w, http.StatusCreated, map[string]any{
		"secret_id":       secret.ID,
		"secret_value":    raw,
		"webhook_url":     baseUrl + "/ingest/" + acct.Slug + "/" + provider + "/" + listenerID + "/" + raw,
		"listener_id":     listenerID,
		"provider":        provider,
		"deployment_mode": parseModeFromTypeKey(whType.TypeKey),
	})
}

func (h *Handler) ListListenerSecrets(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	listenerID := normalizeListenerID(chi.URLParam(r, "listenerID"))
	provider := normalizeProvider(r.URL.Query().Get("provider"))
	if provider == "" || listenerID == "" {
		writeErr(w, http.StatusBadRequest, "provider and listenerID required")
		return
	}
	whType, err := h.findListenerType(r.Context(), h.Store, acct.ID, provider, listenerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	secrets, err := h.Store.ListSecrets(r.Context(), acct.ID, whType.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseUrl := h.publicBaseURL()
	resp := make([]map[string]any, 0, len(secrets))
	for _, sec := range secrets {
		resp = append(resp, map[string]any{
			"id":           sec.ID,
			"status":       sec.Status,
			"created_at":   sec.CreatedAt,
			"secret_value": sec.SecretValue,
			"webhook_url":  baseUrl + "/ingest/" + acct.Slug + "/" + provider + "/" + listenerID + "/" + sec.SecretValue,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteListener(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	listenerID := normalizeListenerID(chi.URLParam(r, "listenerID"))
	provider := normalizeProvider(r.URL.Query().Get("provider"))
	if listenerID == "" {
		writeErr(w, http.StatusBadRequest, "listenerID required")
		return
	}
	// Find the webhook type – provider is helpful but we fall back to any match
	var whType domain.WebhookType
	types, _ := h.Store.ListWebhookTypes(r.Context(), acct.ID)
	for _, t := range types {
		ref, ok := parseListenerTypeKey(t.TypeKey)
		if !ok {
			continue
		}
		if ref.ListenerID == listenerID && (provider == "" || ref.Provider == provider) {
			whType = t
			break
		}
	}
	if whType.ID == "" {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	// Revoke all active secrets belonging to this type
	secrets, _ := h.Store.ListSecrets(r.Context(), acct.ID, whType.ID)
	for _, sec := range secrets {
		_ = h.Store.DeleteSecret(r.Context(), acct.ID, sec.ID)
	}
	if err := h.Store.DeleteWebhookType(r.Context(), acct.ID, whType.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "listener_id": listenerID})
}

func (h *Handler) ListListenerEvents(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	listenerID := normalizeListenerID(chi.URLParam(r, "listenerID"))
	provider := normalizeProvider(r.URL.Query().Get("provider"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	events, err := h.Store.ListEvents(r.Context(), acct.ID, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []map[string]any{}
	for _, ev := range events {
		ref, ok := parseListenerTypeKey(ev.TypeKey)
		if !ok {
			continue
		}
		if listenerID != "" && ref.ListenerID != listenerID {
			continue
		}
		if provider != "" && ref.Provider != provider {
			continue
		}
		out = append(out, map[string]any{
			"event_id":         ev.ID,
			"listener_id":      ref.ListenerID,
			"provider":         ref.Provider,
			"deployment_mode":  ref.DeploymentMode,
			"secret_id":        ev.SecretID,
			"source_event_id":  ev.SourceEventID,
			"raw_payload_json": ev.RawPayloadJSON,
			"payload_json":     ev.PayloadJSON,
			"processed_text":   ev.ProcessedText,
			"action_selected":  ev.ActionSelected,
			"status":           ev.Status,
			"created_at":       ev.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) ReceiveWebhook(w http.ResponseWriter, r *http.Request) {
	accountSlug := chi.URLParam(r, "account")
	typeKey := chi.URLParam(r, "type")
	secretRaw := chi.URLParam(r, "secret")

	acct, err := h.Store.GetAccountBySlug(r.Context(), accountSlug)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	whType, err := h.Store.GetWebhookTypeByAccountAndKey(r.Context(), acct.ID, typeKey)
	if err != nil {
		writeErr(w, http.StatusNotFound, "type not found")
		return
	}
	sec, err := h.Store.ValidateSecret(r.Context(), acct.ID, whType.ID, secretRaw)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid secret")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read payload")
		return
	}
	if h.VerifyHTCSignature {
		if err := verifyHTCSignature(secretRaw, r.Header.Get("X-HTC-Webhook-Timestamp"), r.Header.Get("X-HTC-Webhook-Signature"), buf); err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
	}
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	event, decision, err := h.Processor.ProcessWebhook(r.Context(), acct, whType, sec, requestID, string(buf))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"event": event, "decision": decision})
}

func (h *Handler) ReceiveWebhookByProvider(w http.ResponseWriter, r *http.Request) {
	accountSlug := chi.URLParam(r, "account")
	provider := normalizeProvider(chi.URLParam(r, "provider"))
	webhookID := normalizeListenerID(chi.URLParam(r, "webhookID"))
	secretRaw := chi.URLParam(r, "secret")
	acct, err := h.Store.GetAccountBySlug(r.Context(), accountSlug)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	whType, err := h.findListenerType(r.Context(), h.Store, acct.ID, provider, webhookID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	sec, err := h.Store.ValidateSecret(r.Context(), acct.ID, whType.ID, secretRaw)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid secret")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read payload")
		return
	}
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	event, decision, err := h.Processor.ProcessWebhook(r.Context(), acct, whType, sec, requestID, string(buf))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"listener_id": webhookID,
		"provider":    provider,
		"event":       event,
		"decision":    decision,
	})
}

func (h *Handler) ReceiveWebhookAuto(w http.ResponseWriter, r *http.Request) {
	accountSlug := chi.URLParam(r, "account")
	secretRaw := chi.URLParam(r, "secret")
	acct, err := h.Store.GetAccountBySlug(r.Context(), accountSlug)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	sec, err := h.Store.ResolveSecretAnyType(r.Context(), acct.ID, secretRaw)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid secret")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read payload")
		return
	}
	if h.VerifyHTCSignature {
		if err := verifyHTCSignature(secretRaw, r.Header.Get("X-HTC-Webhook-Timestamp"), r.Header.Get("X-HTC-Webhook-Signature"), buf); err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid webhook signature")
			return
		}
	}
	headers := requestHeaders(r)
	whType, err := h.Store.GetWebhookTypeByID(r.Context(), sec.TypeID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "type not found")
		return
	}
	if h.Processor != nil && h.Processor.IsDeterministicOnlyType(whType.TypeKey) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		event, decision, pErr := h.Processor.ProcessWebhook(r.Context(), acct, whType, sec, requestID, string(buf))
		if pErr != nil {
			writeErr(w, http.StatusInternalServerError, "processing failed: "+pErr.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"event":    event,
			"decision": decision,
			"resolution": domain.TypeResolution{
				TypeKey:      whType.TypeKey,
				Confidence:   1,
				Source:       "deterministic_locked",
				Reason:       "type is configured as deterministic-only",
				ManualReview: false,
			},
		})
		return
	}
	resolution, err := h.Processor.ResolveType(r.Context(), acct.ID, string(buf), headers)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "type resolution failed: "+err.Error())
		return
	}
	if resolution.TypeKey != "" && resolution.TypeKey != "unknown" {
		if matched, mErr := h.Store.GetWebhookTypeByAccountAndKey(r.Context(), acct.ID, resolution.TypeKey); mErr == nil {
			whType = matched
		}
	}
	processCtx := r.Context()
	if strings.TrimSpace(resolution.TypeKey) == "" || resolution.TypeKey == "unknown" {
		// If resolver is uncertain, still process via LLM on full JSON after secret validation.
		whType.PlainTextAction = ""
		whType.UseLLMFallback = true
		processCtx = service.WithSkipTransform(processCtx)
	}
	requestID := r.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	event, decision, err := h.Processor.ProcessWebhook(processCtx, acct, whType, sec, requestID, string(buf))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "processing failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"event": event, "decision": decision, "resolution": resolution})
}

func (h *Handler) CreateSignature(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TypeKey             string          `json:"type_key"`
		Version             int             `json:"version"`
		RequiredKeys        json.RawMessage `json:"required_keys"`
		ShapeHints          json.RawMessage `json:"shape_hints"`
		HeaderHints         json.RawMessage `json:"header_hints"`
		ConfidenceThreshold float64         `json:"confidence_threshold"`
		Enabled             bool            `json:"enabled"`
		Source              string          `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	sig := domain.WebhookTypeSignature{
		AccountID:           acct.ID,
		TypeKey:             strings.TrimSpace(body.TypeKey),
		Version:             body.Version,
		RequiredKeysJSON:    string(body.RequiredKeys),
		ShapeHintsJSON:      string(body.ShapeHints),
		HeaderHintsJSON:     string(body.HeaderHints),
		ConfidenceThreshold: body.ConfidenceThreshold,
		Enabled:             body.Enabled,
		Source:              strings.TrimSpace(body.Source),
	}
	out, err := h.Store.CreateTypeSignature(r.Context(), sig)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) ListSignatures(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.Store.ListTypeSignatures(r.Context(), acct.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) CreateTransform(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TypeKey            string `json:"type_key"`
		Version            int    `json:"version"`
		Engine             string `json:"engine"`
		WASMBlobRef        string `json:"wasm_blob_ref"`
		DSLText            string `json:"dsl_text"`
		DeterministicTests string `json:"deterministic_tests_json"`
		Status             string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	tr, err := h.Store.CreateTransform(r.Context(), domain.WebhookTransform{
		AccountID:              acct.ID,
		TypeKey:                strings.TrimSpace(body.TypeKey),
		Version:                body.Version,
		Engine:                 strings.TrimSpace(body.Engine),
		WASMBlobRef:            strings.TrimSpace(body.WASMBlobRef),
		DSLText:                body.DSLText,
		DeterministicTestsJSON: body.DeterministicTests,
		Status:                 strings.TrimSpace(body.Status),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tr)
}

func (h *Handler) ListTransforms(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	typeKey := strings.TrimSpace(r.URL.Query().Get("type_key"))
	out, err := h.Store.ListTransforms(r.Context(), acct.ID, typeKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) DryRunSkills(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Payload string `json:"payload"`
		TypeKey string `json:"type_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.TypeKey == "" {
		writeErr(w, http.StatusBadRequest, "type_key required")
		return
	}
	skills, err := h.Store.ListWebhookSkills(r.Context(), acct.ID, body.TypeKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	skill, matched := h.Processor.MatchSkill(skills, body.Payload)
	out := map[string]any{
		"matched": matched,
		"skill":   nil,
	}
	if matched {
		out["skill"] = skill
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) DryRunClassify(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read payload")
		return
	}
	out, err := h.Processor.ResolveType(r.Context(), acct.ID, string(buf), requestHeaders(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) ReprocessEvent(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		writeErr(w, http.StatusBadRequest, "eventID required")
		return
	}
	event, decision, err := h.Processor.ReprocessEvent(r.Context(), acct.ID, eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":    event,
		"decision": decision,
	})
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		writeErr(w, http.StatusBadRequest, "eventID required")
		return
	}
	event, err := h.Store.GetEvent(r.Context(), acct.ID, eventID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *Handler) DryRunTransform(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	typeKey := strings.TrimSpace(r.URL.Query().Get("type_key"))
	if typeKey == "" {
		writeErr(w, http.StatusBadRequest, "type_key required")
		return
	}
	buf, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read payload")
		return
	}
	ts := service.NewTransformService(h.Store)
	out, err := ts.Apply(r.Context(), acct.ID, typeKey, string(buf))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) UpsertMasterPromptPolicy(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		PromptText string `json:"prompt_text"`
		UpdatedBy  string `json:"updated_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	out, err := h.Store.UpsertMasterPromptPolicy(r.Context(), acct.ID, strings.TrimSpace(body.PromptText), strings.TrimSpace(body.UpdatedBy))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GetMasterPromptPolicy(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.Store.GetMasterPromptPolicy(r.Context(), acct.ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) CreateWebhookSkill(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		TypeKey         string `json:"type_key"`
		SkillKey        string `json:"skill_key"`
		SkillPrompt     string `json:"skill_prompt"`
		MatchContains   string `json:"match_contains"`
		ForcedAction    string `json:"forced_action"`
		MemoryWriteMode string `json:"memory_write_mode"`
		Priority        int    `json:"priority"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	skill := domain.WebhookSkill{
		AccountID:       acct.ID,
		TypeKey:         strings.TrimSpace(body.TypeKey),
		SkillKey:        strings.TrimSpace(body.SkillKey),
		SkillPrompt:     strings.TrimSpace(body.SkillPrompt),
		MatchContains:   strings.TrimSpace(body.MatchContains),
		ForcedAction:    strings.TrimSpace(body.ForcedAction),
		MemoryWriteMode: strings.TrimSpace(body.MemoryWriteMode),
		Priority:        body.Priority,
		Enabled:         enabled,
	}
	if skill.TypeKey == "" || skill.SkillKey == "" {
		writeErr(w, http.StatusBadRequest, "type_key and skill_key required")
		return
	}
	out, err := h.Store.CreateWebhookSkill(r.Context(), skill)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) ListWebhookSkills(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	typeKey := strings.TrimSpace(r.URL.Query().Get("type_key"))
	if typeKey == "" {
		writeErr(w, http.StatusBadRequest, "type_key required")
		return
	}
	out, err := h.Store.ListWebhookSkills(r.Context(), acct.ID, typeKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) findListenerType(ctx context.Context, st domain.Store, accountID, provider, listenerID string) (domain.WebhookType, error) {
	types, err := st.ListWebhookTypes(ctx, accountID)
	if err != nil {
		return domain.WebhookType{}, err
	}
	for _, item := range types {
		ref, ok := parseListenerTypeKey(item.TypeKey)
		if !ok {
			continue
		}
		if ref.Provider == provider && ref.ListenerID == listenerID {
			return item, nil
		}
	}
	return domain.WebhookType{}, errors.New("listener not found")
}

func parseModeFromTypeKey(typeKey string) string {
	ref, ok := parseListenerTypeKey(typeKey)
	if !ok {
		return "multitenant"
	}
	return ref.DeploymentMode
}

func shortID() string {
	return strings.ReplaceAll(uuid.NewString()[:8], "-", "")
}

func providerTokenTest(ctx context.Context, baseURL, key string) map[string]any {
	if strings.TrimSpace(key) == "" {
		return map[string]any{"ok": false, "error": "missing_api_key"}
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return map[string]any{"ok": false, "error": "missing_base_url"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer resp.Body.Close()
	var payload map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if resp.StatusCode >= 300 {
		return map[string]any{
			"ok":          false,
			"status_code": resp.StatusCode,
			"error":       fmt.Sprintf("provider returned %s", resp.Status),
		}
	}
	modelCount := 0
	if data, ok := payload["data"].([]interface{}); ok {
		modelCount = len(data)
	}
	return map[string]any{
		"ok":          true,
		"status_code": resp.StatusCode,
		"model_count": modelCount,
	}
}

func AuthMiddleware(v auth.RequestVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acct, err := v.VerifyRequest(r)
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), auth.AccountCtxKey, acct)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func requestHeaders(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, vals := range r.Header {
		if len(vals) > 0 {
			out[strings.ToLower(k)] = vals[0]
		}
	}
	return out
}

func verifyHTCSignature(secretRaw, tsRaw, sigRaw string, payload []byte) error {
	tsRaw = strings.TrimSpace(tsRaw)
	sigRaw = strings.TrimSpace(sigRaw)
	if tsRaw == "" && sigRaw == "" {
		return nil
	}
	if tsRaw == "" || sigRaw == "" {
		return errors.New("missing signature headers")
	}
	ts, err := time.Parse(time.RFC3339, tsRaw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if ts.Before(now.Add(-15*time.Minute)) || ts.After(now.Add(5*time.Minute)) {
		return errors.New("timestamp out of allowed window")
	}
	sig := sigRaw
	if strings.HasPrefix(strings.ToLower(sig), "v1=") {
		sig = sig[3:]
	}
	expectedMAC := hmac.New(sha256.New, []byte(secretRaw))
	expectedMAC.Write([]byte(strconv.FormatInt(ts.Unix(), 10)))
	expectedMAC.Write([]byte("."))
	expectedMAC.Write(payload)
	expected := expectedMAC.Sum(nil)
	got, err := hex.DecodeString(sig)
	if err != nil {
		return err
	}
	if !hmac.Equal(got, expected) {
		return errors.New("signature mismatch")
	}
	return nil
}
