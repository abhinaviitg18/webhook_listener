package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
)

type processorCtxKey string

const skipTransformCtxKey processorCtxKey = "skip_transform"

func WithSkipTransform(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipTransformCtxKey, true)
}

func shouldSkipTransform(ctx context.Context) bool {
	v, _ := ctx.Value(skipTransformCtxKey).(bool)
	return v
}

type ActionService struct {
	Telegram *integrations.TelegramClient
	Client   *http.Client
}

func NewActionService(telegram *integrations.TelegramClient) *ActionService {
	return &ActionService{Telegram: telegram, Client: &http.Client{Timeout: 5 * time.Second}}
}

func (a *ActionService) AvailableActions() []string {
	return []string{"store_mysql", "forward_http", "forward_telegram", "no_action"}
}

func (a *ActionService) Execute(ctx context.Context, action domain.ProcessDecision, _ domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	switch action.ActionName {
	case "store_mysql", "no_action", "manual_review":
		return nil
	case "forward_http":
		return a.forwardHTTP(ctx, event.PayloadJSON, targets)
	case "forward_telegram":
		return a.forwardTelegram(ctx, event.PayloadJSON, targets)
	default:
		return fmt.Errorf("unknown action: %s", action.ActionName)
	}
}

func (a *ActionService) forwardHTTP(ctx context.Context, payload string, targets []domain.ForwardTarget) error {
	for _, t := range targets {
		if t.TargetType != "http" {
			continue
		}
		var cfg map[string]string
		_ = json.Unmarshal([]byte(t.ConfigJSON), &cfg)
		url := strings.TrimSpace(cfg["url"])
		if url == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.Client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
	}
	return nil
}

func (a *ActionService) forwardTelegram(ctx context.Context, payload string, targets []domain.ForwardTarget) error {
	if a.Telegram == nil {
		return fmt.Errorf("telegram client unavailable")
	}
	for _, t := range targets {
		if t.TargetType != "telegram" {
			continue
		}
		var cfg map[string]string
		_ = json.Unmarshal([]byte(t.ConfigJSON), &cfg)
		chatID := strings.TrimSpace(cfg["chat_id"])
		if chatID == "" {
			continue
		}
		text := payload
		if len(text) > 3000 {
			text = text[:3000]
		}
		if err := a.Telegram.SendMessage(ctx, chatID, text); err != nil {
			return err
		}
	}
	return nil
}

type Processor struct {
	Store             domain.Store
	Pinecone          domain.PineconeClient
	LLM               domain.LLMClient
	Executor          domain.ActionExecutor
	Resolver          *TypeResolver
	Transformer       *TransformService
	DeterministicOnly map[string]struct{}
	BYOKResolver      func(ctx context.Context, accountID string) domain.LLMClient
}

type memoryWriteMode string

const (
	memoryModeUpdateOrInsert memoryWriteMode = "update_or_insert"
	memoryModeInsertOnly     memoryWriteMode = "insert_only"
	memoryModeNone           memoryWriteMode = "none"
)

type decisionPolicyContext struct {
	MasterPrompt string
	Skills       []domain.WebhookSkill
}

func (p *Processor) MatchSkill(skills []domain.WebhookSkill, payload string) (domain.WebhookSkill, bool) {
	return chooseSkill(skills, payload)
}

func (p *Processor) ProcessWebhook(ctx context.Context, account domain.Account, whType domain.WebhookType, secret domain.WebhookSecret, requestID, payload string) (domain.WebhookEvent, domain.ProcessDecision, error) {
	rawPayload := payload
	sourceEventID := extractSourceEventID(payload)
	if sourceEventID != "" {
		if existing, err := p.Store.FindEventBySourceEventID(ctx, account.ID, sourceEventID); err == nil {
			return existing, domain.ProcessDecision{
				ActionName: "no_action",
				Reason:     "duplicate source event_id",
				Params:     map[string]interface{}{"source_event_id": sourceEventID},
			}, nil
		}
	}

	if p.Transformer != nil && !shouldSkipTransform(ctx) {
		if transformed, err := p.Transformer.Apply(ctx, account.ID, whType.TypeKey, payload); err == nil && strings.TrimSpace(transformed.CanonicalPayload) != "" {
			payload = transformed.CanonicalPayload
			if sourceEventID == "" {
				sourceEventID = extractSourceEventID(payload)
			}
		}
	}

	event, err := p.Store.CreateEvent(ctx, domain.WebhookEvent{
		AccountID:      account.ID,
		TypeID:         whType.ID,
		SecretID:       secret.ID,
		RequestID:      requestID,
		SourceEventID:  sourceEventID,
		TypeKey:        whType.TypeKey,
		RawPayloadJSON: rawPayload,
		PayloadJSON:    payload,
		ProcessedText:  payloadToText(payload),
		Status:         "processing",
	})
	if err != nil {
		return domain.WebhookEvent{}, domain.ProcessDecision{}, err
	}

	return p.processWithPolicy(ctx, account, whType, event, payload)
}

func (p *Processor) ReprocessEvent(ctx context.Context, accountID, eventID string) (domain.WebhookEvent, domain.ProcessDecision, error) {
	event, err := p.Store.GetEvent(ctx, accountID, eventID)
	if err != nil {
		return domain.WebhookEvent{}, domain.ProcessDecision{}, err
	}
	account, err := p.Store.GetAccount(ctx, accountID)
	if err != nil {
		return domain.WebhookEvent{}, domain.ProcessDecision{}, err
	}
	whType, err := p.Store.GetWebhookTypeByAccountAndKey(ctx, accountID, event.TypeKey)
	if err != nil {
		return domain.WebhookEvent{}, domain.ProcessDecision{}, err
	}

	// For re-processing, we use the already stored PayloadJSON (which might be canonically transformed)
	log.Printf("reprocess.start account_id=%s event_id=%s type_key=%s payload_bytes=%d payload_sha256=%x", accountID, eventID, event.TypeKey, len(event.PayloadJSON), payloadFingerprint(event.PayloadJSON))
	return p.processWithPolicy(ctx, account, whType, event, event.PayloadJSON)
}

func (p *Processor) processWithPolicy(ctx context.Context, account domain.Account, whType domain.WebhookType, event domain.WebhookEvent, payload string) (domain.WebhookEvent, domain.ProcessDecision, error) {
	policyCtx := p.loadPolicyContext(ctx, account.ID, whType.TypeKey)
	memories := []domain.PineconeMemory{}
	if p.Pinecone != nil {
		memories, _ = p.Pinecone.Query(ctx, account.ID, payload, 5)
	}

	decision := domain.ProcessDecision{ActionName: "store_mysql", Reason: "default", Params: map[string]interface{}{}}
	skill, matched := chooseSkill(policyCtx.Skills, payload)
	log.Printf("reprocess.policy event_id=%s type_key=%s matched_skill=%t skill_key=%s payload_bytes=%d payload_sha256=%x skill_count=%d memories=%d", event.ID, whType.TypeKey, matched, skill.SkillKey, len(payload), payloadFingerprint(payload), len(policyCtx.Skills), len(memories))
	if matched && strings.TrimSpace(skill.ForcedAction) != "" {
		decision.ActionName = strings.TrimSpace(skill.ForcedAction)
		decision.Reason = "skill:" + skill.SkillKey
		decision.Params = map[string]interface{}{
			"skill_key":          skill.SkillKey,
			"memory_write_mode":  normalizeMemoryMode(skill.MemoryWriteMode),
			"skill_match_tokens": skill.MatchContains,
		}
	} else if strings.TrimSpace(whType.PlainTextAction) != "" {
		decision.ActionName = strings.TrimSpace(whType.PlainTextAction)
		decision.Reason = "type plain text action"
	}

	needsLLM := (whType.UseLLMFallback && decision.ActionName == "store_mysql") || policyCtx.MasterPrompt != "" || (matched && skill.SkillPrompt != "")
	log.Printf("reprocess.llm_decision event_id=%s type_key=%s needs_llm=%t deterministic_only=%t use_llm_fallback=%t matched_skill_prompt=%t master_prompt=%t initial_action=%s", event.ID, whType.TypeKey, needsLLM, p.IsDeterministicOnlyType(whType.TypeKey), whType.UseLLMFallback, matched && strings.TrimSpace(skill.SkillPrompt) != "", policyCtx.MasterPrompt != "", decision.ActionName)
	if needsLLM && !p.IsDeterministicOnlyType(whType.TypeKey) {
		// Resolve LLM client: prefer per-account BYOK key, fall back to global
		llmClient := p.LLM
		if p.BYOKResolver != nil {
			if byokLLM := p.BYOKResolver(ctx, account.ID); byokLLM != nil {
				llmClient = byokLLM
			}
		}
		if llmClient != nil {
			llmPayload := payload
			if policyCtx.MasterPrompt != "" || len(policyCtx.Skills) > 0 {
				llmPayload = buildPolicyAwarePayload(payload, policyCtx)
			}
			d, derr := llmClient.SuggestAction(ctx, whType.TypeKey, llmPayload, memories, p.Executor.AvailableActions())
			if derr == nil {
				if matched && strings.TrimSpace(skill.ForcedAction) != "" {
					// Deterministic action wins, but use LLM's processed text
					decision.ProcessedText = d.ProcessedText
					decision.Tags = d.Tags
				} else {
					decision = d
				}
			} else {
				decision.Reason = "llm processing failed: " + derr.Error()
				log.Printf("reprocess.llm_error event_id=%s type_key=%s err=%v", event.ID, whType.TypeKey, derr)
			}
		}
	}

	targets, _ := p.Store.ListForwardTargets(ctx, account.ID)
	err := p.Executor.Execute(ctx, decision, account, event, targets)
	if err != nil {
		_ = p.Store.UpdateEventStatus(ctx, event.ID, "failed", decision.ActionName)
		log.Printf("reprocess.execute_failed event_id=%s type_key=%s action=%s err=%v", event.ID, whType.TypeKey, decision.ActionName, err)
		return event, decision, err
	}
	_ = p.Store.UpdateEventStatus(ctx, event.ID, "processed", decision.ActionName)

	if p.Pinecone != nil {
		mode := memoryModeFromDecision(decision, skill)
		switch mode {
		case memoryModeNone:
			// Explicitly skip vector write.
		case memoryModeInsertOnly:
			_ = p.Pinecone.UpsertOrUpdate(ctx, account.ID, whType.TypeKey, event.ID, payload, nil)
		default:
			_ = p.Pinecone.UpsertOrUpdate(ctx, account.ID, whType.TypeKey, event.ID, payload, memories)
		}
	}
	event.Status = "processed"
	event.ActionSelected = decision.ActionName
	if strings.TrimSpace(decision.ProcessedText) != "" {
		event.ProcessedText = decision.ProcessedText
	} else {
		event.ProcessedText = payloadToText(payload)
	}
	if len(decision.Tags) > 0 {
		tagsBytes, _ := json.Marshal(decision.Tags)
		event.TagsJSON = string(tagsBytes)
		_ = p.Store.UpdateEventTags(ctx, event.ID, event.TagsJSON)
	}
	event.CreatedAt = time.Now().UTC()
	log.Printf("reprocess.complete event_id=%s type_key=%s action=%s reason=%q processed_text_bytes=%d tags=%d", event.ID, whType.TypeKey, decision.ActionName, decision.Reason, len(event.ProcessedText), len(decision.Tags))
	return event, decision, nil
}

func payloadFingerprint(payload string) [32]byte {
	return sha256.Sum256([]byte(payload))
}

func (p *Processor) IsDeterministicOnlyType(typeKey string) bool {
	if p == nil {
		return false
	}
	return isDeterministicOnly(typeKey, p.DeterministicOnly)
}

func payloadToText(payload string) string {
	trimmed := strings.TrimSpace(payload)
	if len(trimmed) <= 1200 {
		return trimmed
	}
	return trimmed[:1200]
}

func (p *Processor) ResolveType(ctx context.Context, accountID, payload string, headers map[string]string) (domain.TypeResolution, error) {
	if p.Resolver == nil {
		return domain.TypeResolution{TypeKey: "unknown", Confidence: 0, Source: "none", Reason: "resolver unavailable", ManualReview: false}, nil
	}
	return p.Resolver.Resolve(ctx, accountID, payload, headers)
}

func (p *Processor) loadPolicyContext(ctx context.Context, accountID, typeKey string) decisionPolicyContext {
	out := decisionPolicyContext{}
	if p.Store == nil {
		return out
	}
	if pol, err := p.Store.GetMasterPromptPolicy(ctx, accountID); err == nil {
		out.MasterPrompt = strings.TrimSpace(pol.PromptText)
	}
	skills, err := p.Store.ListWebhookSkills(ctx, accountID, typeKey)
	if err == nil {
		out.Skills = skills
	}
	return out
}

func chooseSkill(skills []domain.WebhookSkill, payload string) (domain.WebhookSkill, bool) {
	if len(skills) == 0 {
		return domain.WebhookSkill{}, false
	}
	normalized := strings.ToLower(payload)
	ordered := append([]domain.WebhookSkill{}, skills...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Priority == ordered[j].Priority {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].Priority < ordered[j].Priority
	})
	for _, sk := range ordered {
		if !sk.Enabled {
			continue
		}
		if matchesAnyToken(normalized, sk.MatchContains) {
			return sk, true
		}
	}
	return domain.WebhookSkill{}, false
}

func matchesAnyToken(normalizedPayload, matchContains string) bool {
	tokens := strings.Split(matchContains, ",")
	for _, t := range tokens {
		token := strings.TrimSpace(strings.ToLower(t))
		if token == "" {
			continue
		}
		if strings.Contains(normalizedPayload, token) {
			return true
		}
	}
	return false
}

func normalizeMemoryMode(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case string(memoryModeInsertOnly):
		return string(memoryModeInsertOnly)
	case string(memoryModeNone):
		return string(memoryModeNone)
	default:
		return string(memoryModeUpdateOrInsert)
	}
}

func memoryModeFromDecision(d domain.ProcessDecision, sk domain.WebhookSkill) memoryWriteMode {
	if d.Params != nil {
		if raw, ok := d.Params["memory_write_mode"]; ok {
			if s, ok := raw.(string); ok {
				return memoryWriteMode(normalizeMemoryMode(s))
			}
		}
	}
	return memoryWriteMode(normalizeMemoryMode(sk.MemoryWriteMode))
}

func buildPolicyAwarePayload(payload string, policyCtx decisionPolicyContext) string {
	skillViews := make([]map[string]string, 0, len(policyCtx.Skills))
	for _, sk := range policyCtx.Skills {
		skillViews = append(skillViews, map[string]string{
			"skill_key":         sk.SkillKey,
			"skill_prompt":      sk.SkillPrompt,
			"match_contains":    sk.MatchContains,
			"forced_action":     sk.ForcedAction,
			"memory_write_mode": normalizeMemoryMode(sk.MemoryWriteMode),
		})
	}
	b, _ := json.Marshal(map[string]interface{}{
		"master_prompt": policyCtx.MasterPrompt,
		"skills":        skillViews,
		"payload":       payload,
	})
	return string(b)
}

func extractSourceEventID(payload string) string {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return ""
	}
	for _, key := range []string{"event_id", "provider_message_id", "request_id"} {
		if v := strings.TrimSpace(asString(root[key])); v != "" {
			return v
		}
	}
	if msg, ok := root["message"].(map[string]interface{}); ok {
		for _, key := range []string{"provider_message_id", "id"} {
			if v := strings.TrimSpace(asString(msg[key])); v != "" {
				return v
			}
		}
	}
	return ""
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}
