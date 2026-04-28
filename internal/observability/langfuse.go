package observability

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	Enabled   bool
	Host      string
	PublicKey string
	SecretKey string
}

type Client interface {
	StartLLMDecision(ctx context.Context, meta LLMDecisionMetadata) LLMDecisionTrace
}

type LLMDecisionTrace interface {
	StartAttempt(provider, model string) LLMDecisionAttempt
	Finish(result LLMDecisionResult)
}

type LLMDecisionAttempt interface {
	Finish(result LLMAttemptResult)
}

type LLMDecisionMetadata struct {
	TraceID               string
	EventID               string
	AccountHash           string
	TypeKey               string
	Operation             string
	MatchedSkillKey       string
	PayloadHash           string
	PayloadBytes          int
	CompactedPayloadBytes int
	UsedCompaction        bool
	FallbackChainSize     int
}

type LLMDecisionResult struct {
	FinalAction         string
	DecisionReason      string
	WinningProvider     string
	WinningModel        string
	Outcome             string
	UsedFallback        bool
	ProducedTags        bool
	ProcessedTextSource string
	TagsCount           int
	ErrorClass          string
	ErrorStatus         int
}

type LLMAttemptResult struct {
	Provider         string
	Model            string
	Outcome          string
	ErrorClass       string
	ErrorStatus      int
	StatusMessage    string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type langfuseClient struct {
	host       string
	authHeader string
	client     *http.Client
}

type noopClient struct{}
type noopDecisionTrace struct{}
type noopDecisionAttempt struct{}

type langfuseDecisionTrace struct {
	client          *langfuseClient
	traceID         string
	rootSpanID      string
	events          []map[string]any
	startedAt       time.Time
	meta            LLMDecisionMetadata
	mu              sync.Mutex
	attemptCount    int
	winningProvider string
	winningModel    string
}

type langfuseDecisionAttempt struct {
	parent    *langfuseDecisionTrace
	id        string
	provider  string
	model     string
	startedAt time.Time
}

type ctxKey string

const traceCtxKey ctxKey = "langfuse_llm_trace"

type traceContextState struct {
	trace           LLMDecisionTrace
	winningProvider string
	winningModel    string
	attemptCount    int
	winningAttempt  int
}

func NewLangfuseClient(cfg Config) Client {
	if !cfg.Enabled || strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.PublicKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return noopClient{}
	}
	auth := base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(cfg.PublicKey) + ":" + strings.TrimSpace(cfg.SecretKey)))
	return &langfuseClient{
		host:       strings.TrimRight(strings.TrimSpace(cfg.Host), "/"),
		authHeader: "Basic " + auth,
		client:     &http.Client{Timeout: 8 * time.Second},
	}
}

func WithLLMTrace(ctx context.Context, trace LLMDecisionTrace) context.Context {
	return context.WithValue(ctx, traceCtxKey, &traceContextState{trace: trace})
}

func TraceFromContext(ctx context.Context) LLMDecisionTrace {
	state, _ := ctx.Value(traceCtxKey).(*traceContextState)
	if state == nil {
		return noopDecisionTrace{}
	}
	return state.trace
}

func MarkWinningAttempt(ctx context.Context, provider, model string) {
	state, _ := ctx.Value(traceCtxKey).(*traceContextState)
	if state == nil {
		return
	}
	state.winningProvider = provider
	state.winningModel = model
	state.winningAttempt = state.attemptCount
}

func WinningAttemptFromContext(ctx context.Context) (string, string) {
	state, _ := ctx.Value(traceCtxKey).(*traceContextState)
	if state == nil {
		return "", ""
	}
	return state.winningProvider, state.winningModel
}

func RecordAttemptStart(ctx context.Context) {
	state, _ := ctx.Value(traceCtxKey).(*traceContextState)
	if state == nil {
		return
	}
	state.attemptCount++
}

func FallbackUsedFromContext(ctx context.Context) bool {
	state, _ := ctx.Value(traceCtxKey).(*traceContextState)
	if state == nil {
		return false
	}
	return state.winningAttempt > 1
}

func HashIdentifier(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

func (noopClient) StartLLMDecision(context.Context, LLMDecisionMetadata) LLMDecisionTrace {
	return noopDecisionTrace{}
}

func (noopDecisionTrace) StartAttempt(string, string) LLMDecisionAttempt {
	return noopDecisionAttempt{}
}
func (noopDecisionTrace) Finish(LLMDecisionResult)  {}
func (noopDecisionAttempt) Finish(LLMAttemptResult) {}

func (c *langfuseClient) StartLLMDecision(_ context.Context, meta LLMDecisionMetadata) LLMDecisionTrace {
	traceID := meta.TraceID
	if strings.TrimSpace(traceID) == "" {
		traceID = uuid.NewString()
	}
	rootSpanID := uuid.NewString()
	now := time.Now().UTC()
	trace := &langfuseDecisionTrace{
		client:     c,
		traceID:    traceID,
		rootSpanID: rootSpanID,
		startedAt:  now,
		meta:       meta,
	}
	trace.events = append(trace.events,
		trace.ingestionEvent("trace-create", map[string]any{
			"id":          traceID,
			"timestamp":   now.Format(time.RFC3339Nano),
			"name":        "llm.decision",
			"sessionId":   meta.EventID,
			"userId":      meta.AccountHash,
			"environment": meta.Operation,
			"metadata": map[string]any{
				"event_id":                meta.EventID,
				"type_key":                meta.TypeKey,
				"operation":               meta.Operation,
				"matched_skill_key":       meta.MatchedSkillKey,
				"payload_hash":            meta.PayloadHash,
				"payload_bytes":           meta.PayloadBytes,
				"compacted_payload_bytes": meta.CompactedPayloadBytes,
				"used_compaction":         meta.UsedCompaction,
				"fallback_chain_size":     meta.FallbackChainSize,
			},
			"tags": []string{"agenthook", "llm", meta.Operation},
		}),
		trace.ingestionEvent("span-create", map[string]any{
			"id":        rootSpanID,
			"traceId":   traceID,
			"name":      "llm.decision",
			"startTime": now.Format(time.RFC3339Nano),
			"metadata": map[string]any{
				"event_id":          meta.EventID,
				"type_key":          meta.TypeKey,
				"matched_skill_key": meta.MatchedSkillKey,
			},
			"environment": meta.Operation,
		}),
	)
	return trace
}

func (t *langfuseDecisionTrace) StartAttempt(provider, model string) LLMDecisionAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attemptCount++
	now := time.Now().UTC()
	id := uuid.NewString()
	t.events = append(t.events, t.ingestionEvent("generation-create", map[string]any{
		"id":                  id,
		"traceId":             t.traceID,
		"parentObservationId": t.rootSpanID,
		"name":                "llm.provider_attempt",
		"startTime":           now.Format(time.RFC3339Nano),
		"model":               model,
		"metadata": map[string]any{
			"provider": provider,
			"attempt":  t.attemptCount,
		},
		"environment": t.meta.Operation,
	}))
	return &langfuseDecisionAttempt{
		parent:    t,
		id:        id,
		provider:  provider,
		model:     model,
		startedAt: now,
	}
}

func (t *langfuseDecisionTrace) Finish(result LLMDecisionResult) {
	t.mu.Lock()
	t.events = append(t.events, t.ingestionEvent("span-update", map[string]any{
		"id":      t.rootSpanID,
		"traceId": t.traceID,
		"endTime": time.Now().UTC().Format(time.RFC3339Nano),
		"metadata": map[string]any{
			"winning_provider":      result.WinningProvider,
			"winning_model":         result.WinningModel,
			"outcome":               result.Outcome,
			"used_fallback":         result.UsedFallback,
			"final_action":          result.FinalAction,
			"decision_reason":       result.DecisionReason,
			"processed_text_source": result.ProcessedTextSource,
			"produced_tags":         result.ProducedTags,
			"tags_count":            result.TagsCount,
			"error_class":           result.ErrorClass,
			"error_status":          result.ErrorStatus,
		},
		"statusMessage": result.Outcome,
	}))
	events := append([]map[string]any(nil), t.events...)
	t.mu.Unlock()
	t.client.flush(events)
}

func (a *langfuseDecisionAttempt) Finish(result LLMAttemptResult) {
	a.parent.mu.Lock()
	defer a.parent.mu.Unlock()
	if result.Outcome == "success" {
		a.parent.winningProvider = result.Provider
		a.parent.winningModel = result.Model
	}
	body := map[string]any{
		"id":      a.id,
		"traceId": a.parent.traceID,
		"endTime": time.Now().UTC().Format(time.RFC3339Nano),
		"model":   result.Model,
		"metadata": map[string]any{
			"provider":     result.Provider,
			"outcome":      result.Outcome,
			"error_class":  result.ErrorClass,
			"error_status": result.ErrorStatus,
		},
		"statusMessage": result.StatusMessage,
	}
	if result.TotalTokens > 0 || result.PromptTokens > 0 || result.CompletionTokens > 0 {
		body["usage"] = map[string]any{
			"promptTokens":     result.PromptTokens,
			"completionTokens": result.CompletionTokens,
			"totalTokens":      result.TotalTokens,
		}
	}
	a.parent.events = append(a.parent.events, a.parent.ingestionEvent("generation-update", body))
}

func (t *langfuseDecisionTrace) ingestionEvent(eventType string, body map[string]any) map[string]any {
	return map[string]any{
		"id":        uuid.NewString(),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"type":      eventType,
		"body":      body,
	}
}

func (c *langfuseClient) flush(events []map[string]any) {
	if len(events) == 0 {
		return
	}
	payload := map[string]any{
		"batch": events,
		"metadata": map[string]any{
			"source": "agenthook",
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		log.Printf("langfuse.flush marshal_failed err=%v", err)
		return
	}
	req, err := http.NewRequest(http.MethodPost, c.host+"/api/public/ingestion", bytes.NewReader(b))
	if err != nil {
		log.Printf("langfuse.flush request_failed err=%v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("langfuse.flush do_failed err=%v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusMultiStatus {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		log.Printf("langfuse.flush failed status=%s body=%q", resp.Status, strings.TrimSpace(string(body)))
		return
	}
}
