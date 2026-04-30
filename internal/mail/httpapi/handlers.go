package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/config"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/mail"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Service      *mail.Service
	AccountStore domain.Store
}

func NewRouter(h *Handler, verifier auth.RequestVerifier) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	r.Group(func(ar chi.Router) {
		ar.Use(authMiddleware(verifier))
		ar.Get("/v1/mailboxes", h.ListMailboxes)
		ar.Get("/v1/mailboxes/{mailboxID}/messages", h.ListMailboxMessages)
		ar.Post("/v1/mailboxes/{mailboxID}/send", h.SendFromMailbox)
		ar.Get("/v1/messages/{messageID}", h.GetMessage)
		ar.Post("/v1/messages/{messageID}/reply", h.ReplyToMessage)
	})
	r.Post("/internal/ingress/s3-event", h.IngestS3Event)
	return r
}

func authMiddleware(v auth.RequestVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acct, err := v.VerifyRequest(r)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), auth.AccountCtxKey, acct)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (h *Handler) ListMailboxes(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := h.Service.SyncMailboxes(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) ListMailboxMessages(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mailboxID := chi.URLParam(r, "mailboxID")
	items, err := h.Service.Store.ListMessages(r.Context(), acct.ID, mailboxID, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	messageID := chi.URLParam(r, "messageID")
	message, attachments, err := h.Service.Store.GetMessage(r.Context(), acct.ID, messageID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":     message,
		"attachments": attachments,
	})
}

func (h *Handler) SendFromMailbox(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body mail.SendRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.FromMailboxID = chi.URLParam(r, "mailboxID")
	out, err := h.Service.SendMessage(r.Context(), acct.ID, body.FromMailboxID, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) ReplyToMessage(w http.ResponseWriter, r *http.Request) {
	acct, ok := auth.AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body mail.SendRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	out, err := h.Service.ReplyToMessage(r.Context(), acct.ID, chi.URLParam(r, "messageID"), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) IngestS3Event(w http.ResponseWriter, r *http.Request) {
	if sharedSecret := strings.TrimSpace(LoadEnv().MailInternalSharedSecret); sharedSecret != "" {
		if strings.TrimSpace(r.Header.Get("x-agenthook-mail-secret")) != sharedSecret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	var body mail.InboundS3Event
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	results := make([]map[string]any, 0, len(body.Records))
	for _, record := range body.Records {
		message, err := h.Service.IngestS3Object(r.Context(), record.S3.Bucket.Name, record.S3.Object.Key)
		result := map[string]any{
			"bucket": record.S3.Bucket.Name,
			"key":    record.S3.Object.Key,
		}
		if err != nil {
			result["error"] = err.Error()
		} else {
			result["message_id"] = message.ID
			result["account_id"] = message.AccountID
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func LoadEnv() config.Config {
	_ = config.LoadEnvFiles("local.env", ".env")
	cfg := config.Load()
	cfg.MailDomain = strings.TrimSpace(cfg.MailDomain)
	return cfg
}
