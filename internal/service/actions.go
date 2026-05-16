package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/integrations"
	"agenthook.store/internal/observability"
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
	Telegram  *integrations.TelegramClient
	Client    *http.Client
	Store     domain.Store
	LookupEnv func(string) (string, bool)
}

func NewActionService(telegram *integrations.TelegramClient) *ActionService {
	return &ActionService{Telegram: telegram, Client: &http.Client{Timeout: 5 * time.Second}, LookupEnv: os.LookupEnv}
}

func (a *ActionService) AvailableActions() []string {
	return []string{"store_mysql", "forward_http", "forward_telegram", "slack_notify", "crm_upsert", "ticket_create", "no_action", "manual_review"}
}

func (a *ActionService) Execute(ctx context.Context, action domain.ProcessDecision, account domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	switch action.ActionName {
	case "store_mysql", "no_action", "manual_review":
		return nil
	case "forward_http":
		return a.forwardHTTP(ctx, action, account, event, targets)
	case "forward_telegram":
		return a.forwardTelegram(ctx, action, account, event, targets)
	case "slack_notify":
		return a.notifyIntegration(ctx, action, account, event, targets)
	case "crm_upsert", "ticket_create":
		return a.postStructuredIntegration(ctx, action, account, event, targets)
	default:
		return fmt.Errorf("unknown action: %s", action.ActionName)
	}
}

func (a *ActionService) forwardHTTP(ctx context.Context, action domain.ProcessDecision, account domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	payload := event.PayloadJSON
	targetKey, _ := action.Params["integration_target_key"].(string)
	targetKey = strings.TrimSpace(targetKey)
	if targetKey != "" {
		target, ok := resolveIntegrationTarget(targets, targetKey, action.ActionName)
		if !ok {
			return fmt.Errorf("integration target not found: %s", targetKey)
		}
		return a.postJSONToHTTP(ctx, account, event, HydrateForwardTarget(target), payload)
	}
	for _, t := range targets {
		target := HydrateForwardTarget(t)
		if target.TargetType != "http" || !target.Enabled {
			continue
		}
		if err := a.postJSONToHTTP(ctx, account, event, target, payload); err != nil {
			return err
		}
	}
	return nil
}

func (a *ActionService) forwardTelegram(ctx context.Context, action domain.ProcessDecision, account domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	if a.Telegram == nil {
		return fmt.Errorf("telegram client unavailable")
	}
	targetKey, _ := action.Params["integration_target_key"].(string)
	targetKey = strings.TrimSpace(targetKey)
	if targetKey == "" {
		for _, raw := range targets {
			target := HydrateForwardTarget(raw)
			if target.TargetType != "telegram" || !target.Enabled {
				continue
			}
			if err := a.sendTelegramMessage(ctx, account, event, target, buildIntegrationEnvelope(action, event)); err != nil {
				return err
			}
		}
		return nil
	}
	target, ok := resolveIntegrationTarget(targets, targetKey, action.ActionName)
	if !ok {
		return fmt.Errorf("integration target not found: %s", targetKey)
	}
	return a.sendTelegramMessage(ctx, account, event, HydrateForwardTarget(target), buildIntegrationEnvelope(action, event))
}

func (a *ActionService) notifyIntegration(ctx context.Context, action domain.ProcessDecision, account domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	targetKey, _ := action.Params["integration_target_key"].(string)
	target, ok := resolveIntegrationTarget(targets, strings.TrimSpace(targetKey), action.ActionName)
	if !ok {
		return fmt.Errorf("integration target not found: %s", targetKey)
	}
	target = HydrateForwardTarget(target)
	message := buildNotificationMessage(action, event)
	switch target.TargetType {
	case "telegram":
		return a.sendTelegramMessage(ctx, account, event, target, message)
	case "http":
		return a.postJSONToHTTP(ctx, account, event, target, buildIntegrationEnvelope(action, event))
	default:
		return fmt.Errorf("unsupported target type for slack_notify: %s", target.TargetType)
	}
}

func (a *ActionService) postStructuredIntegration(ctx context.Context, action domain.ProcessDecision, account domain.Account, event domain.WebhookEvent, targets []domain.ForwardTarget) error {
	targetKey, _ := action.Params["integration_target_key"].(string)
	target, ok := resolveIntegrationTarget(targets, strings.TrimSpace(targetKey), action.ActionName)
	if !ok {
		return fmt.Errorf("integration target not found: %s", targetKey)
	}
	target = HydrateForwardTarget(target)
	if target.TargetType != "http" {
		return fmt.Errorf("structured integration target must be http, got %s", target.TargetType)
	}
	return a.postJSONToHTTP(ctx, account, event, target, buildIntegrationEnvelope(action, event))
}

func (a *ActionService) sendTelegramMessage(ctx context.Context, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, text string) error {
	if a.Telegram == nil {
		return fmt.Errorf("telegram client unavailable")
	}
	cfg := parseIntegrationTargetConfig(target.TargetType, target.ConfigJSON)
	resolved, err := a.resolveTargetConfig(ctx, account, event, target, cfg)
	if err != nil {
		return err
	}
	chatID, _ := resolved["chat_id"].(string)
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return fmt.Errorf("telegram target %s missing chat_id", target.TargetKey)
	}
	if len(text) > 3000 {
		text = text[:3000]
	}
	return a.Telegram.SendMessage(ctx, chatID, text)
}

func (a *ActionService) postJSONToHTTP(ctx context.Context, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, payload string) error {
	cfg := parseIntegrationTargetConfig(target.TargetType, target.ConfigJSON)
	req, err := a.buildHTTPRequest(ctx, account, event, target, cfg, payload)
	if err != nil {
		return err
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (a *ActionService) buildHTTPRequest(ctx context.Context, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, cfg integrationTargetConfig, payload string) (*http.Request, error) {
	resolvedConfig, err := a.resolveTargetConfig(ctx, account, event, target, cfg)
	if err != nil {
		return nil, err
	}
	rawURL, _ := resolvedConfig["url"].(string)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("http target %s missing url", target.TargetKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AgentHook-Forwarder/1.0")
	if headers, ok := resolvedConfig["headers"].(map[string]interface{}); ok {
		for key, raw := range headers {
			value, _ := raw.(string)
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			resolvedValue, rErr := a.resolveLegacyHeaderTemplate(target, value)
			if rErr != nil {
				return nil, rErr
			}
			req.Header.Set(key, resolvedValue)
		}
	}
	if err := a.applyAuthToRequest(ctx, req, account, event, target, cfg); err != nil {
		return nil, err
	}
	return req, nil
}

func (a *ActionService) resolveTargetConfig(ctx context.Context, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, cfg integrationTargetConfig) (map[string]interface{}, error) {
	resolved := map[string]interface{}{}
	for key, value := range cfg.Config {
		resolved[key] = value
	}
	for headerName, secretRef := range cfg.HeaderSecretRefs {
		value, source, err := a.resolveSecretValue(ctx, account, event, target, strings.TrimSpace(secretRef), "")
		if err != nil {
			log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=secret_ref status=failed reason=%v", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey), err)
			return nil, err
		}
		headers := getOrCreateConfigMap(resolved, "headers")
		headers[headerName] = value
		log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=%s status=resolved reason=header_secret_ref", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey), source)
	}
	for headerName, envVar := range cfg.HeaderEnvRefs {
		value, ok := a.lookupEnv(strings.TrimSpace(envVar))
		if !ok || strings.TrimSpace(value) == "" {
			log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=env status=failed reason=missing_env_var", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey))
			return nil, fmt.Errorf("env var %s not found for header %s", envVar, headerName)
		}
		headers := getOrCreateConfigMap(resolved, "headers")
		headers[headerName] = value
		log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=explicit_env status=resolved reason=header_env_ref", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey))
	}
	return resolved, nil
}

func (a *ActionService) applyAuthToRequest(ctx context.Context, req *http.Request, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, cfg integrationTargetConfig) error {
	authType := strings.TrimSpace(strings.ToLower(cfg.Auth.Type))
	if authType == "" {
		return nil
	}
	value, source, err := a.resolveSecretValue(ctx, account, event, target, strings.TrimSpace(cfg.Auth.SecretRef), strings.TrimSpace(cfg.Auth.EnvVar))
	if err != nil {
		log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=auth status=failed reason=%v", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey), err)
		return err
	}
	switch authType {
	case "bearer_header":
		headerName := strings.TrimSpace(cfg.Auth.HeaderName)
		if headerName == "" {
			headerName = "Authorization"
		}
		prefix := cfg.Auth.Prefix
		if prefix == "" {
			prefix = "Bearer "
		}
		req.Header.Set(headerName, prefix+value)
	case "custom_header":
		headerName := strings.TrimSpace(cfg.Auth.HeaderName)
		if headerName == "" {
			return fmt.Errorf("custom_header auth requires header_name")
		}
		req.Header.Set(headerName, cfg.Auth.Prefix+value)
	case "query_param":
		param := strings.TrimSpace(cfg.Auth.QueryParam)
		if param == "" {
			return fmt.Errorf("query_param auth requires query_param")
		}
		u, err := url.Parse(req.URL.String())
		if err != nil {
			return err
		}
		q := u.Query()
		q.Set(param, value)
		u.RawQuery = q.Encode()
		req.URL = u
	default:
		return fmt.Errorf("unsupported auth type: %s", authType)
	}
	log.Printf("integration.secret_resolution target_key=%s target_type=%s deployment_mode=%s source_kind=%s status=resolved reason=auth", target.TargetKey, target.TargetType, deploymentModeFromTypeKey(event.TypeKey), source)
	return nil
}

func (a *ActionService) resolveSecretValue(ctx context.Context, account domain.Account, event domain.WebhookEvent, target domain.ForwardTarget, secretRef, explicitEnvVar string) (string, string, error) {
	if strings.TrimSpace(secretRef) != "" && a.Store != nil {
		value, err := a.Store.ResolveIntegrationSecretValue(ctx, account.ID, secretRef)
		if err == nil && strings.TrimSpace(value) != "" {
			return value, "secret_ref", nil
		}
	}
	if deploymentModeFromTypeKey(event.TypeKey) == "single_tenant" {
		for _, candidate := range envFallbackCandidates(event.TypeKey, target, secretRef) {
			if value, ok := a.lookupEnv(candidate); ok && strings.TrimSpace(value) != "" {
				return value, "env_fallback", nil
			}
		}
	}
	if strings.TrimSpace(explicitEnvVar) != "" {
		if value, ok := a.lookupEnv(strings.TrimSpace(explicitEnvVar)); ok && strings.TrimSpace(value) != "" {
			return value, "explicit_env", nil
		}
	}
	if strings.TrimSpace(secretRef) != "" {
		return "", "", fmt.Errorf("missing_secret_ref")
	}
	if strings.TrimSpace(explicitEnvVar) != "" {
		return "", "", fmt.Errorf("missing_env_var")
	}
	return "", "", fmt.Errorf("missing_auth_secret")
}

func (a *ActionService) resolveLegacyHeaderTemplate(target domain.ForwardTarget, value string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	start := strings.Index(value, "${")
	end := strings.Index(value[start:], "}")
	if start < 0 || end <= 1 {
		return value, nil
	}
	end = start + end
	envVar := strings.TrimSpace(value[start+2 : end])
	envValue, ok := a.lookupEnv(envVar)
	if !ok || strings.TrimSpace(envValue) == "" {
		return "", fmt.Errorf("env var %s not found for legacy header template on %s", envVar, target.TargetKey)
	}
	return strings.ReplaceAll(value, "${"+envVar+"}", envValue), nil
}

func (a *ActionService) lookupEnv(key string) (string, bool) {
	if a.LookupEnv == nil {
		return os.LookupEnv(key)
	}
	return a.LookupEnv(key)
}

func getOrCreateConfigMap(config map[string]interface{}, key string) map[string]interface{} {
	if existing, ok := config[key].(map[string]interface{}); ok {
		return existing
	}
	next := map[string]interface{}{}
	config[key] = next
	return next
}

func deploymentModeFromTypeKey(typeKey string) string {
	parts := strings.Split(strings.TrimSpace(typeKey), "::")
	if len(parts) >= 4 {
		mode := strings.TrimSpace(strings.ToLower(parts[len(parts)-1]))
		if mode == "single_tenant" {
			return "single_tenant"
		}
	}
	return "multitenant"
}

func envFallbackCandidates(typeKey string, target domain.ForwardTarget, secretRef string) []string {
	provider := ""
	parts := strings.Split(strings.TrimSpace(typeKey), "::")
	if len(parts) >= 2 {
		provider = parts[1]
	}
	candidates := []string{}
	appendCandidate := func(raw string) {
		raw = normalizeEnvKey(raw)
		if raw == "" {
			return
		}
		for _, suffix := range []string{"API_KEY", "TOKEN"} {
			candidate := raw
			if !strings.HasSuffix(candidate, "_"+suffix) {
				candidate = candidate + "_" + suffix
			}
			for _, existing := range candidates {
				if existing == candidate {
					return
				}
			}
			candidates = append(candidates, candidate)
		}
	}
	appendCandidate(secretRef)
	appendCandidate(target.TargetKey)
	appendCandidate(trimCommonTargetSuffix(target.TargetKey))
	appendCandidate(target.TargetType)
	appendCandidate(provider)
	return candidates
}

func normalizeEnvKey(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	if raw == "" {
		return ""
	}
	replacer := strings.NewReplacer("-", "_", " ", "_", ".", "_", ":", "_")
	raw = replacer.Replace(raw)
	for strings.Contains(raw, "__") {
		raw = strings.ReplaceAll(raw, "__", "_")
	}
	return strings.Trim(raw, "_")
}

func trimCommonTargetSuffix(raw string) string {
	parts := strings.Split(normalizeEnvKey(raw), "_")
	if len(parts) <= 1 {
		return normalizeEnvKey(raw)
	}
	last := parts[len(parts)-1]
	switch last {
	case "PRIMARY", "DEFAULT", "PROD", "PRODUCTION", "STAGING", "DEV":
		return strings.Join(parts[:len(parts)-1], "_")
	default:
		return strings.Join(parts, "_")
	}
}

func buildIntegrationEnvelope(action domain.ProcessDecision, event domain.WebhookEvent) string {
	payloadValue := asJSONValue(event.PayloadJSON)
	rawPayloadValue := asJSONValue(event.RawPayloadJSON)
	payload := map[string]interface{}{
		"action_name":    action.ActionName,
		"reason":         action.Reason,
		"params":         action.Params,
		"processed_text": action.ProcessedText,
		"tags":           action.Tags,
		"event": map[string]interface{}{
			"id":               event.ID,
			"type_key":         event.TypeKey,
			"request_id":       event.RequestID,
			"source_event_id":  event.SourceEventID,
			"payload_json":     payloadValue,
			"raw_payload_json": rawPayloadValue,
		},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func buildNotificationMessage(action domain.ProcessDecision, event domain.WebhookEvent) string {
	if raw, ok := action.Params["message_fields"].(map[string]interface{}); ok && len(raw) > 0 {
		b, _ := json.Marshal(raw)
		return string(b)
	}
	if strings.TrimSpace(action.ProcessedText) != "" {
		return action.ProcessedText
	}
	if strings.TrimSpace(event.ProcessedText) != "" {
		return event.ProcessedText
	}
	return payloadToText(event.PayloadJSON)
}

func asJSONValue(raw string) interface{} {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var out interface{}
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out
	}
	return trimmed
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
	LLMCompaction     LLMCompactionConfig
	Tracer            observability.Client
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
	workingPayload := sanitizePayloadForProcessing(payload)

	event, err := p.Store.CreateEvent(ctx, domain.WebhookEvent{
		AccountID:      account.ID,
		TypeID:         whType.ID,
		SecretID:       secret.ID,
		RequestID:      requestID,
		SourceEventID:  sourceEventID,
		TypeKey:        whType.TypeKey,
		RawPayloadJSON: rawPayload,
		PayloadJSON:    payload,
		ProcessedText:  payloadToText(workingPayload),
		Status:         "processing",
	})
	if err != nil {
		return domain.WebhookEvent{}, domain.ProcessDecision{}, err
	}

	return p.processWithPolicy(ctx, account, whType, event, payload, workingPayload, "process")
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
	return p.processWithPolicy(ctx, account, whType, event, event.PayloadJSON, sanitizePayloadForProcessing(event.PayloadJSON), "reprocess")
}

func (p *Processor) processWithPolicy(ctx context.Context, account domain.Account, whType domain.WebhookType, event domain.WebhookEvent, payload string, workingPayload string, operation string) (domain.WebhookEvent, domain.ProcessDecision, error) {
	policyCtx := p.loadPolicyContext(ctx, account.ID, whType.TypeKey)
	memories := []domain.PineconeMemory{}
	if p.Pinecone != nil {
		memories, _ = p.Pinecone.Query(ctx, account.ID, workingPayload, 5)
	}
	targets, _ := p.Store.ListForwardTargets(ctx, account.ID)

	decision := domain.ProcessDecision{ActionName: "store_mysql", Reason: "default", Params: map[string]interface{}{}}
	route := routeEvent(ctx, p.LLM, whType.TypeKey, workingPayload, policyCtx.Skills)
	selectedSkills := selectSkillsForExecution(policyCtx.Skills, route.SelectedSkillKeys)
	if len(selectedSkills) == 0 && len(route.Candidates) > 0 {
		selectedSkills = []domain.WebhookSkill{route.Candidates[0].Skill}
	}
	skill, matched := chooseSkill(selectedSkills, workingPayload)
	log.Printf("reprocess.policy event_id=%s type_key=%s matched_skill=%t skill_key=%s payload_bytes=%d payload_sha256=%x skill_count=%d memories=%d route=%s", event.ID, whType.TypeKey, matched, skill.SkillKey, len(payload), payloadFingerprint(payload), len(policyCtx.Skills), len(memories), routeLogSummary(route))
	if route.SpamLabel == spamLabelSpam && len(selectedSkills) == 0 {
		decision.ActionName = "no_action"
		decision.Reason = "deterministic spam gate"
	}
	explicitPlainTextAction := strings.TrimSpace(whType.PlainTextAction)
	if matched && strings.TrimSpace(skill.ForcedAction) != "" {
		decision.ActionName = strings.TrimSpace(skill.ForcedAction)
		decision.Reason = "skill:" + skill.SkillKey
		decision.Params = map[string]interface{}{
			"skill_key":          skill.SkillKey,
			"memory_write_mode":  normalizeMemoryMode(skill.MemoryWriteMode),
			"skill_match_tokens": skill.MatchContains,
		}
	} else if explicitPlainTextAction != "" {
		decision.ActionName = explicitPlainTextAction
		decision.Reason = "type plain text action"
	}
	if route.CandidateAction != "" && decision.ActionName == "store_mysql" {
		decision.ActionName = route.CandidateAction
		decision.Reason = "router:" + route.CandidateAction
		decision.Params["integration_target_key"] = route.IntegrationTargetKey
	}

	activePolicy := policyCtx
	activePolicy.Skills = selectedSkills
	hasSkillPrompt := matched && strings.TrimSpace(skill.SkillPrompt) != ""
	hasLockedPlainTextAction := explicitPlainTextAction != "" && !whType.UseLLMFallback && !hasSkillPrompt
	needsLLM := !hasLockedPlainTextAction && ((whType.UseLLMFallback && decision.ActionName == "store_mysql") ||
		activePolicy.MasterPrompt != "" ||
		hasSkillPrompt ||
		(route.CandidateAction != "" && !(matched && strings.TrimSpace(skill.ForcedAction) != "")))
	log.Printf("reprocess.llm_decision event_id=%s type_key=%s needs_llm=%t deterministic_only=%t use_llm_fallback=%t matched_skill_prompt=%t master_prompt=%t initial_action=%s", event.ID, whType.TypeKey, needsLLM, p.IsDeterministicOnlyType(whType.TypeKey), whType.UseLLMFallback, matched && strings.TrimSpace(skill.SkillPrompt) != "", policyCtx.MasterPrompt != "", decision.ActionName)
	var llmTrace observability.LLMDecisionTrace
	if needsLLM && !p.IsDeterministicOnlyType(whType.TypeKey) {
		// Resolve LLM client: prefer per-account BYOK key, fall back to global
		llmClient := p.LLM
		if p.BYOKResolver != nil {
			if byokLLM := p.BYOKResolver(ctx, account.ID); byokLLM != nil {
				llmClient = byokLLM
			}
		}
		if llmClient != nil {
			compaction := compactPayloadForLLM(workingPayload, p.LLMCompaction)
			if p.Tracer != nil {
				llmTrace = p.Tracer.StartLLMDecision(ctx, observability.LLMDecisionMetadata{
					TraceID:               event.ID + "-llm",
					EventID:               event.ID,
					AccountHash:           observability.HashIdentifier(account.ID),
					TypeKey:               whType.TypeKey,
					Operation:             operation,
					MatchedSkillKey:       skill.SkillKey,
					PayloadHash:           fmt.Sprintf("%x", payloadFingerprint(payload)),
					PayloadBytes:          len(payload),
					CompactedPayloadBytes: compaction.CompactedBytes,
					UsedCompaction:        compaction.WasCompacted,
					FallbackChainSize:     llmFallbackChainSize(llmClient),
				})
				ctx = observability.WithLLMTrace(ctx, llmTrace)
			}
			llmPayloadSource := compaction.CompactedPayload
			if activePolicy.MasterPrompt != "" || len(activePolicy.Skills) > 0 {
				llmPayloadSource = buildPolicyAwarePayload(compaction.CompactedPayload, activePolicy)
			}
			ratio := 1.0
			if compaction.OriginalBytes > 0 {
				ratio = float64(compaction.CompactedBytes) / float64(compaction.OriginalBytes)
			}
			log.Printf(
				"reprocess.llm_payload event_id=%s type_key=%s compacted=%t original_bytes=%d compacted_bytes=%d ratio=%.3f dropped_fields=%d truncated_strings=%d truncated_arrays=%d",
				event.ID,
				whType.TypeKey,
				compaction.WasCompacted,
				compaction.OriginalBytes,
				compaction.CompactedBytes,
				ratio,
				compaction.DroppedFields,
				compaction.TruncatedStrings,
				compaction.TruncatedArrays,
			)
			d, derr := llmClient.SuggestAction(ctx, whType.TypeKey, llmPayloadSource, memories, p.Executor.AvailableActions())
			if derr == nil {
				if matched && strings.TrimSpace(skill.ForcedAction) != "" {
					// Deterministic action wins, but use LLM's processed text
					decision.ProcessedText = d.ProcessedText
					decision.Tags = d.Tags
					if route.IntegrationTargetKey != "" {
						decision.Params["integration_target_key"] = route.IntegrationTargetKey
					}
				} else {
					decision = d
					if route.IntegrationTargetKey != "" {
						if decision.Params == nil {
							decision.Params = map[string]interface{}{}
						}
						if _, ok := decision.Params["integration_target_key"]; !ok {
							decision.Params["integration_target_key"] = route.IntegrationTargetKey
						}
					}
				}
			} else {
				decision.Reason = "llm processing failed: " + derr.Error()
				log.Printf("reprocess.llm_error event_id=%s type_key=%s err=%v", event.ID, whType.TypeKey, derr)
			}
		}
	}

	normalizedDecision, validationErr := validateAndNormalizeDecision(decision, targets)
	if validationErr != nil {
		log.Printf("reprocess.action_validation_failed event_id=%s type_key=%s action=%s err=%v", event.ID, whType.TypeKey, decision.ActionName, validationErr)
		if normalizedDecision.ActionName != "store_mysql" && normalizedDecision.ActionName != "no_action" {
			decision = newManualReviewDecision(validationErr.Error())
		}
	} else {
		decision = normalizedDecision
	}
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
			_ = p.Pinecone.UpsertOrUpdate(ctx, account.ID, whType.TypeKey, event.ID, workingPayload, nil)
		default:
			_ = p.Pinecone.UpsertOrUpdate(ctx, account.ID, whType.TypeKey, event.ID, workingPayload, memories)
		}
	}
	event.Status = "processed"
	event.ActionSelected = decision.ActionName
	if strings.TrimSpace(decision.ProcessedText) != "" {
		event.ProcessedText = decision.ProcessedText
	} else {
		event.ProcessedText = payloadToText(workingPayload)
	}
	if err := p.Store.UpdateEventProcessedText(ctx, event.ID, event.ProcessedText); err != nil {
		log.Printf("reprocess.persist_processed_text_failed event_id=%s type_key=%s err=%v", event.ID, whType.TypeKey, err)
	}
	if len(decision.Tags) > 0 {
		tagsBytes, _ := json.Marshal(decision.Tags)
		event.TagsJSON = string(tagsBytes)
		if err := p.Store.UpdateEventTags(ctx, event.ID, event.TagsJSON); err != nil {
			log.Printf("reprocess.persist_tags_failed event_id=%s type_key=%s err=%v", event.ID, whType.TypeKey, err)
		}
	}
	event.CreatedAt = time.Now().UTC()
	if llmTrace != nil {
		winningProvider, winningModel := observability.WinningAttemptFromContext(ctx)
		llmTrace.Finish(observability.LLMDecisionResult{
			FinalAction:         decision.ActionName,
			DecisionReason:      decision.Reason,
			WinningProvider:     winningProvider,
			WinningModel:        winningModel,
			Outcome:             llmOutcome(decision),
			UsedFallback:        observability.FallbackUsedFromContext(ctx),
			ProducedTags:        len(decision.Tags) > 0,
			ProcessedTextSource: processedTextSource(decision, event.ProcessedText),
			TagsCount:           len(decision.Tags),
			ErrorClass:          llmErrorClass(decision.Reason),
		})
	}
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

func llmFallbackChainSize(client domain.LLMClient) int {
	switch c := client.(type) {
	case *integrations.FallbackLLMClient:
		return len(c.Clients)
	default:
		if client != nil {
			return 1
		}
		return 0
	}
}

func llmOutcome(decision domain.ProcessDecision) string {
	if strings.HasPrefix(decision.Reason, "llm processing failed:") {
		return "provider_error"
	}
	if strings.TrimSpace(decision.ProcessedText) != "" || len(decision.Tags) > 0 {
		return "success"
	}
	return "fallback"
}

func processedTextSource(decision domain.ProcessDecision, stored string) string {
	if strings.TrimSpace(decision.ProcessedText) != "" {
		return "llm"
	}
	if strings.TrimSpace(stored) != "" {
		return "payload_fallback"
	}
	return "none"
}

func llmErrorClass(reason string) string {
	if strings.HasPrefix(reason, "llm processing failed:") {
		return "provider_error"
	}
	return ""
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
