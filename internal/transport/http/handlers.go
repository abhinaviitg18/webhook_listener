package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/domain"
	"hookweb.club/internal/service"
)

type Handler struct {
	Store              domain.Store
	Processor          *service.Processor
	VerifyHTCSignature bool
	ScaleKitBaseURL    string
}

func NewRouter(h *Handler, verifier auth.RequestVerifier) http.Handler {
	r := chi.NewRouter()

	r.Get("/", h.SampleWebsite)
	r.Get("/app", h.PlatformWebsite)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	r.Post("/api/register/email", h.RegisterEmail)
	r.Get("/auth/scalekit/login", h.ScaleKitLoginRedirect)
	r.Get("/auth/scalekit/signup", h.ScaleKitSignupRedirect)

	r.Group(func(ar chi.Router) {
		ar.Use(AuthMiddleware(verifier))
		ar.Post("/api/webhooks/types", h.CreateType)
		ar.Get("/api/webhooks/types", h.ListTypes)
		ar.Post("/api/webhooks/secrets", h.CreateSecret)
		ar.Delete("/api/webhooks/secrets/{secretID}", h.DeleteSecret)
		ar.Post("/api/forward-targets", h.CreateForwardTarget)
		ar.Get("/api/events", h.ListEvents)
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

		ar.Post("/v1/listeners", h.CreateListener)
		ar.Get("/v1/listeners", h.ListListeners)
		ar.Post("/v1/listeners/{listenerID}/secrets", h.CreateListenerSecret)
		ar.Get("/v1/listeners/{listenerID}/events", h.ListListenerEvents)
		ar.Post("/v1/auth/tokens", h.CreateAPIToken)
		ar.Post("/v1/presets/webhook-processing", h.ApplyWebhookProcessingPreset)
		ar.Post("/v1/byok/providers/test", h.TestBYOKProviders)
	})

	r.Post("/url/{account}/{type}/{secret}", h.ReceiveWebhook)
	r.Post("/url/{account}/{secret}", h.ReceiveWebhookAuto)
	r.Post("/ingest/{account}/{provider}/{webhookID}/{secret}", h.ReceiveWebhookByProvider)

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

func (h *Handler) ScaleKitLoginRedirect(w http.ResponseWriter, r *http.Request) {
	base := h.scalekitBase()
	if strings.Contains(base, "scalekit.dev") {
		http.Redirect(w, r, base, http.StatusFound)
		return
	}
	http.Redirect(w, r, strings.TrimRight(base, "/")+"/login", http.StatusFound)
}

func (h *Handler) ScaleKitSignupRedirect(w http.ResponseWriter, r *http.Request) {
	base := h.scalekitBase()
	if strings.Contains(base, "scalekit.dev") {
		u := strings.TrimRight(base, "/") + "/oauth/authorize?screen_hint=signup"
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	http.Redirect(w, r, strings.TrimRight(base, "/")+"/signup", http.StatusFound)
}

func (h *Handler) scalekitBase() string {
	base := strings.TrimSpace(h.ScaleKitBaseURL)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("SCALEKIT_BASE_URL"))
	}
	if base == "" {
		base = "https://www.scalekit.com"
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return strings.TrimRight(base, "/")
	}
	return "https://www.scalekit.com"
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
	whType, err := findListenerType(r.Context(), h.Store, acct.ID, provider, listenerID)
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
	writeJSON(w, http.StatusCreated, map[string]any{
		"listener_id":      listenerID,
		"provider":         provider,
		"deployment_mode":  deploymentMode,
		"type_key":         whType.TypeKey,
		"secret_id":        secret.ID,
		"secret_value":     raw,
		"webhook_url":      "/ingest/" + acct.Slug + "/" + provider + "/" + listenerID + "/" + raw,
		"legacy_webhook":   "/url/" + acct.Slug + "/" + whType.TypeKey + "/" + raw,
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
		resp = append(resp, map[string]any{
			"listener_id":       ref.ListenerID,
			"provider":          ref.Provider,
			"deployment_mode":   ref.DeploymentMode,
			"type_key":          item.TypeKey,
			"plain_text_action": item.PlainTextAction,
			"use_llm_fallback":  item.UseLLMFallback,
			"created_at":        item.CreatedAt,
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
	whType, err := findListenerType(r.Context(), h.Store, acct.ID, provider, listenerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "listener not found")
		return
	}
	secret, raw, err := h.Store.CreateSecret(r.Context(), acct.ID, whType.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"secret_id":       secret.ID,
		"secret_value":    raw,
		"webhook_url":     "/ingest/" + acct.Slug + "/" + provider + "/" + listenerID + "/" + raw,
		"listener_id":     listenerID,
		"provider":        provider,
		"deployment_mode": parseModeFromTypeKey(whType.TypeKey),
	})
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
	whType, err := findListenerType(r.Context(), h.Store, acct.ID, provider, webhookID)
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

func (h *Handler) SampleWebsite(w http.ResponseWriter, _ *http.Request) {
	html := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>hookweb.club</title><style>
:root{--ink:#0f172a;--muted:#475569;--card:#fff;--line:#cbd5e1;--accent:#0f766e;--soft:#e6fffb}
*{box-sizing:border-box}body{margin:0;font-family:ui-sans-serif,system-ui,sans-serif;color:var(--ink);background:radial-gradient(1000px 480px at 8% -12%,#ccfbf1,transparent),radial-gradient(900px 440px at 100% 0%,#dbeafe,transparent),#f8fafc}
main{max-width:1020px;margin:28px auto;padding:0 16px}
section{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:20px;margin-bottom:12px}
h1{margin:8px 0 10px;font-size:40px;line-height:1.02}h2{margin:0 0 10px;font-size:22px}
p{margin:0 0 10px;color:var(--muted);font-size:16px}
.pill{display:inline-block;background:var(--accent);color:#fff;padding:6px 12px;border-radius:999px;font-size:12px;font-weight:700}
.actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:12px}
.btn{display:inline-flex;align-items:center;justify-content:center;height:40px;padding:0 14px;border-radius:10px;border:1px solid var(--line);text-decoration:none;font-weight:700}
.btn.primary{background:var(--accent);border-color:var(--accent);color:#fff}
.btn.secondary{background:#fff;color:var(--accent)}
.story{display:grid;grid-template-columns:1fr 1fr;gap:12px}.block{background:#f8fafc;border:1px dashed #cbd5e1;border-radius:12px;padding:12px}
code{background:var(--soft);padding:2px 6px;border-radius:6px}
ul{margin:0;padding-left:18px;color:#1e293b}
@media (max-width:860px){h1{font-size:32px}.story{grid-template-columns:1fr}}
</style></head><body><main>
<section>
  <span class='pill'>Open Core Webhook Engine</span>
  <h1>hookweb.club</h1>
  <p>Most teams are flooded by webhooks but cannot trust or act on them fast enough. Payloads stay scattered in logs, every integration behaves differently, and one broken secret can halt operations.</p>
  <p><b>Why this is required:</b> we give each producer a secure URL with its own secret, capture every payload, and turn it into human-readable processed text so non-tech teams can monitor and act without waiting for engineering.</p>
  <div class="actions">
    <a class="btn primary" href="/auth/scalekit/signup">Register with ScaleKit</a>
    <a class="btn secondary" href="/auth/scalekit/login">Login with ScaleKit</a>
    <a class="btn secondary" href="/app">Open Dashboard</a>
  </div>
</section>
<section class="story">
  <div class="block">
    <h2>The Problem</h2>
    <ul>
      <li>Slack, Jira, AgentMail, and others all send different JSON shapes.</li>
      <li>Ops teams see failures only after customers are impacted.</li>
      <li>Raw webhook bodies are hard to read and impossible to triage quickly.</li>
    </ul>
  </div>
  <div class="block">
    <h2>The Hookweb Story</h2>
    <ul>
      <li>Every provider gets its own endpoint + secret.</li>
      <li>Every event is stored as <code>raw_payload_json</code> and <code>processed_text</code>.</li>
      <li>Events are grouped by secret for clear source-level audit trails.</li>
    </ul>
  </div>
</section>
<section>
  <h2>How It Works</h2>
  <p>Ingress format: <code>/ingest/{account}/{provider}/{webhook_id}/{secret}</code></p>
  <ul>
    <li>Secret is validated before any processing starts.</li>
    <li>Deterministic logic runs first; LLM fallback is optional by type.</li>
    <li>Actions and forwarding remain auditable per event.</li>
    <li>Ready for <code>normal_plan</code> (multitenant) and <code>enterprise</code> (single_tenant) deployment modes.</li>
  </ul>
</section>
</main></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (h *Handler) PlatformWebsite(w http.ResponseWriter, _ *http.Request) {
	html := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>hookweb.club dashboard</title>
<style>
:root{--bg:#eef6f5;--ink:#0f172a;--muted:#475569;--card:#fff;--line:#c9d9d7;--sea:#115e59;--teal:#0f766e;--rose:#9f1239}
*{box-sizing:border-box}body{margin:0;font-family:ui-sans-serif,system-ui,sans-serif;background:radial-gradient(900px 460px at 8% -8%,#cffafe,transparent),radial-gradient(900px 460px at 102% 0%,#fce7f3,transparent),var(--bg);color:var(--ink)}
main{max-width:1200px;margin:22px auto;padding:0 14px}
.hero,.panel,.secret,.event{background:var(--card);border:1px solid var(--line);border-radius:14px}
.hero{padding:16px;display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between}
.hero h1{margin:0;font-size:28px}.hero p{margin:6px 0 0;color:var(--muted)}
.auth-btns{display:flex;gap:8px;flex-wrap:wrap}
.auth-btns a{display:inline-flex;align-items:center;justify-content:center;height:34px;padding:0 12px;border-radius:9px;text-decoration:none;border:1px solid var(--line);font-size:13px;font-weight:700}
.auth-btns a.primary{background:var(--sea);border-color:var(--sea);color:#fff}
.auth-btns a.secondary{background:#fff;color:var(--sea)}
.row{display:grid;grid-template-columns:2fr 1.2fr;gap:12px;margin-top:12px}
.panel{padding:12px}
.controls{display:grid;grid-template-columns:1.2fr 1.2fr 1fr auto;gap:8px;align-items:center}
input,select,button{height:36px;border-radius:10px;border:1px solid var(--line);padding:0 10px;font-size:14px}
button{background:var(--sea);color:#fff;font-weight:600;border-color:var(--sea);cursor:pointer}
button.alt{background:#fff;color:var(--sea)}
.subpanel{margin-top:10px;padding-top:10px;border-top:1px dashed #d4e6e3}
.subpanel h4{margin:0 0 8px;font-size:14px}
.listeners{max-height:260px;overflow:auto}
.listener{padding:10px;border:1px dashed #d4e6e3;border-radius:10px;margin-bottom:8px;cursor:pointer}
.listener b{display:block}.listener small{color:var(--muted)}
.right pre,.event pre{margin:0;background:#082f31;color:#d1fae5;border-radius:10px;padding:10px;overflow:auto;font-size:12px}
.secrets{display:grid;grid-template-columns:1fr;gap:10px}
.secret{padding:10px}
.secret h4{margin:0 0 8px;font-size:14px;color:var(--rose)}
.event{padding:10px;margin-bottom:8px}
.meta{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:8px}
.pill{display:inline-block;background:#ecfeff;color:#155e75;border:1px solid #bae6fd;border-radius:999px;padding:3px 8px;font-size:12px}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:8px}
@media (max-width:980px){.row{grid-template-columns:1fr}.controls{grid-template-columns:1fr 1fr}.grid2{grid-template-columns:1fr}}
</style></head><body><main>
<section class="hero"><div><h1>hookweb.club Listener Dashboard</h1><p>AgentMail-inspired webhook inbox: grouped by <b>secret</b>, with raw payload and processed text side by side.</p></div><div><div class="pill">Auth: Local token or ScaleKit Bearer</div><div class="auth-btns" style="margin-top:8px"><a class="primary" href="/auth/scalekit/signup">ScaleKit Signup</a><a class="secondary" href="/auth/scalekit/login">ScaleKit Login</a></div></div></section>
<section class="row">
  <section class="panel left">
    <div class="controls">
      <input id="token" placeholder="Bearer token">
      <input id="provider" value="agentmail" placeholder="provider">
      <input id="listenerId" placeholder="listener id (optional)">
      <button id="createBtn">Create Listener</button>
    </div>
    <div style="margin-top:8px" class="controls">
      <select id="mode"><option value="normal_plan">normal_plan (multitenant)</option><option value="enterprise">enterprise (single_tenant)</option></select>
      <input id="action" value="store_mysql" placeholder="plain_text_action">
      <button id="refreshBtn" class="alt">Refresh Listeners</button>
      <button id="newSecretBtn" class="alt">New Secret</button>
    </div>
    <div class="subpanel">
      <h4>Auth + Token</h4>
      <div class="controls">
        <input id="newTokenOut" placeholder="new API token appears here" readonly>
        <button id="createTokenBtn" class="alt">Create API Token</button>
        <a href="/auth/scalekit/signup" style="display:inline-flex;align-items:center;justify-content:center;height:36px;border:1px solid var(--line);border-radius:10px;text-decoration:none;color:var(--sea);font-weight:600;background:#fff">ScaleKit Signup</a>
        <a href="/auth/scalekit/login" style="display:inline-flex;align-items:center;justify-content:center;height:36px;border:1px solid var(--line);border-radius:10px;text-decoration:none;color:var(--sea);font-weight:600;background:#fff">ScaleKit Login</a>
      </div>
    </div>
    <div class="subpanel">
      <h4>Prompt + Skills Preset</h4>
      <div class="controls">
        <input id="generalPrompt" value="Always preserve raw payload. Apply deterministic routing first." placeholder="general prompt">
        <input id="specificPrompt" value="Extract provider fields and store concise processed text." placeholder="specific prompt">
        <input id="specificMatch" value="agentmail" placeholder="specific match tokens">
        <button id="applyPresetBtn" class="alt">Apply Preset</button>
      </div>
    </div>
    <div class="subpanel">
      <h4>BYOK Test (Groq + Cerebras)</h4>
      <div class="controls">
        <input id="groqKey" placeholder="GROQ API key (BYOK)">
        <input id="cerebrasKey" placeholder="Cerebras API key (BYOK)">
        <button id="testByokBtn" class="alt">Test BYOK Keys</button>
        <input id="byokResult" placeholder="BYOK test result" readonly>
      </div>
    </div>
    <div id="status" style="margin-top:8px;color:var(--muted);font-size:13px">Ready.</div>
    <h3 style="margin:12px 0 8px">Listeners</h3>
    <div id="listeners" class="listeners"></div>
  </section>
  <section class="panel right">
    <h3 style="margin:0 0 8px">Selected Listener</h3>
    <pre id="selectedMeta">{}</pre>
    <p style="margin:8px 0 4px;color:var(--muted)">Latest ingest URL</p>
    <pre id="selectedURL">-</pre>
  </section>
</section>
<section class="panel" style="margin-top:12px">
  <h3 style="margin:0 0 8px">Webhook Events Grouped By Secret</h3>
  <div id="eventsBySecret" class="secrets"></div>
</section>
</main>
<script>
const state = {listeners: [], selected: null, latestURL: ""};
let preferredListener = "";
const el = (id) => document.getElementById(id);
const api = async (path, method="GET", body=null) => {
  const token = el("token").value.trim();
  const res = await fetch(path, {
    method,
    headers: Object.assign({"Content-Type":"application/json"}, token ? {"Authorization":"Bearer "+token} : {}),
    body: body ? JSON.stringify(body) : undefined
  });
  const text = await res.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = {raw:text}; }
  if (!res.ok) throw new Error((data && data.error) ? data.error : ("HTTP "+res.status));
  return data;
};
function setStatus(msg){ el("status").textContent = msg; }
function renderListeners(){
  const box = el("listeners"); box.innerHTML = "";
  if (!state.listeners.length){ box.innerHTML = "<div class='listener'>No listeners yet.</div>"; return; }
  state.listeners.forEach((l) => {
    const d = document.createElement("div");
    d.className = "listener";
    d.innerHTML = "<b>"+l.listener_id+"</b><small>"+l.provider+" | "+l.deployment_mode+" | "+l.type_key+"</small>";
    d.onclick = () => selectListener(l);
    box.appendChild(d);
  });
}
function groupBySecret(events){
  const out = {};
  (events || []).forEach((e) => {
    const sid = e.secret_id || "unknown-secret";
    if (!out[sid]) out[sid] = [];
    out[sid].push(e);
  });
  return out;
}
function renderGroupedEvents(events){
  const root = el("eventsBySecret");
  root.innerHTML = "";
  const grouped = groupBySecret(events);
  const secrets = Object.keys(grouped);
  if (!secrets.length){ root.innerHTML = "<div class='secret'>No events for this listener.</div>"; return; }
  secrets.forEach((sid) => {
    const sec = document.createElement("div");
    sec.className = "secret";
    sec.innerHTML = "<h4>Secret: "+sid+" ("+grouped[sid].length+" events)</h4>";
    grouped[sid].forEach((e) => {
      const ev = document.createElement("div");
      ev.className = "event";
      ev.innerHTML = "<div class='meta'>"
        + "<span class='pill'>"+(e.status || "")+"</span>"
        + "<span class='pill'>action: "+(e.action_selected || "-")+"</span>"
        + "<span class='pill'>source_event: "+(e.source_event_id || "-")+"</span>"
        + "</div>"
        + "<div class='grid2'>"
        + "<div><small>Raw JSON</small><pre>"+(e.raw_payload_json || "{}")+"</pre></div>"
        + "<div><small>Processed Text</small><pre>"+(e.processed_text || "")+"</pre></div>"
        + "</div>";
      sec.appendChild(ev);
    });
    root.appendChild(sec);
  });
}
async function loadListeners(){
  try{
    const data = await api("/v1/listeners");
    state.listeners = Array.isArray(data) ? data : [];
    renderListeners();
    setStatus("Loaded "+state.listeners.length+" listeners.");
    if (preferredListener) {
      const found = state.listeners.find((l) => l.listener_id === preferredListener);
      if (found) {
        preferredListener = "";
        selectListener(found);
      }
    }
  }catch(err){ setStatus("Load failed: "+err.message); }
}
async function selectListener(listener){
  state.selected = listener;
  el("selectedMeta").textContent = JSON.stringify(listener, null, 2);
  el("listenerId").value = listener.listener_id;
  el("provider").value = listener.provider;
  el("selectedURL").textContent = state.latestURL || "-";
  try{
    const events = await api("/v1/listeners/"+encodeURIComponent(listener.listener_id)+"/events?provider="+encodeURIComponent(listener.provider)+"&limit=100");
    renderGroupedEvents(events);
    setStatus("Loaded "+events.length+" events for "+listener.listener_id);
  }catch(err){ setStatus("Events load failed: "+err.message); }
}
el("createBtn").onclick = async () => {
  try{
    const body = {
      provider: el("provider").value.trim(),
      listener_id: el("listenerId").value.trim(),
      deployment_mode: el("mode").value,
      plain_text_action: el("action").value.trim(),
      use_llm_fallback: false
    };
    const created = await api("/v1/listeners", "POST", body);
    state.latestURL = created.webhook_url || "";
    el("selectedURL").textContent = state.latestURL || "-";
    setStatus("Listener created: "+created.listener_id);
    await loadListeners();
  }catch(err){ setStatus("Create failed: "+err.message); }
};
el("newSecretBtn").onclick = async () => {
  if (!state.selected){ setStatus("Select listener first."); return; }
  try{
    const out = await api("/v1/listeners/"+encodeURIComponent(state.selected.listener_id)+"/secrets", "POST", {provider: state.selected.provider});
    state.latestURL = out.webhook_url || "";
    el("selectedURL").textContent = state.latestURL || "-";
    setStatus("New secret created: "+out.secret_id);
  }catch(err){ setStatus("Create secret failed: "+err.message); }
};
el("createTokenBtn").onclick = async () => {
  try{
    const out = await api("/v1/auth/tokens", "POST", {});
    el("newTokenOut").value = out.token || "";
    setStatus("Created new API token.");
  }catch(err){ setStatus("Create token failed: "+err.message); }
};
el("applyPresetBtn").onclick = async () => {
  if (!state.selected){ setStatus("Select listener first."); return; }
  try{
    await api("/v1/presets/webhook-processing", "POST", {
      provider: state.selected.provider,
      listener_id: state.selected.listener_id,
      general_prompt: el("generalPrompt").value,
      specific_prompt: el("specificPrompt").value,
      specific_match_contains: el("specificMatch").value,
      specific_action: "store_mysql",
      memory_write_mode: "update_or_insert"
    });
    setStatus("Applied general + specific processing preset.");
  }catch(err){ setStatus("Preset apply failed: "+err.message); }
};
el("testByokBtn").onclick = async () => {
  try{
    const out = await api("/v1/byok/providers/test", "POST", {
      groq_api_key: el("groqKey").value.trim(),
      cerebras_api_key: el("cerebrasKey").value.trim()
    });
    el("byokResult").value = JSON.stringify(out.providers || {});
    setStatus("BYOK provider test completed.");
  }catch(err){ setStatus("BYOK test failed: "+err.message); }
};
el("refreshBtn").onclick = loadListeners;
const params = new URLSearchParams(window.location.search);
if (params.get("token")) el("token").value = params.get("token");
if (params.get("provider")) el("provider").value = params.get("provider");
if (params.get("listener")) el("listenerId").value = params.get("listener");
preferredListener = params.get("listener") || "";
loadListeners();
</script></body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func findListenerType(ctx context.Context, st domain.Store, accountID, provider, listenerID string) (domain.WebhookType, error) {
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
