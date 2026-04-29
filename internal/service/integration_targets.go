package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"agenthook.store/internal/domain"
)

type integrationTargetConfig struct {
	TargetKey      string                 `json:"target_key"`
	Purpose        string                 `json:"purpose"`
	Enabled        *bool                  `json:"enabled,omitempty"`
	AllowedActions []string               `json:"allowed_actions,omitempty"`
	Schema         map[string]interface{} `json:"schema,omitempty"`
	Config         map[string]interface{} `json:"config,omitempty"`
}

var allowedActionFamilies = map[string]struct{}{
	"store_mysql":      {},
	"no_action":        {},
	"manual_review":    {},
	"forward_http":     {},
	"forward_telegram": {},
	"slack_notify":     {},
	"crm_upsert":       {},
	"ticket_create":    {},
}

func HydrateForwardTarget(target domain.ForwardTarget) domain.ForwardTarget {
	cfg := parseIntegrationTargetConfig(target.TargetType, target.ConfigJSON)
	target.TargetKey = cfg.TargetKey
	target.Purpose = cfg.Purpose
	target.Enabled = cfg.Enabled == nil || *cfg.Enabled
	if len(cfg.AllowedActions) > 0 {
		b, _ := json.Marshal(cfg.AllowedActions)
		target.AllowedActionsJSON = string(b)
	}
	if len(cfg.Schema) > 0 {
		b, _ := json.Marshal(cfg.Schema)
		target.SchemaJSON = string(b)
	}
	if target.TargetKey == "" {
		target.TargetKey = target.ID
	}
	return target
}

func parseIntegrationTargetConfig(targetType, raw string) integrationTargetConfig {
	cfg := integrationTargetConfig{
		TargetKey: strings.TrimSpace(targetType),
		Config:    map[string]interface{}{},
	}
	if strings.TrimSpace(raw) == "" {
		return cfg
	}
	_ = json.Unmarshal([]byte(raw), &cfg)
	if cfg.Config == nil {
		cfg.Config = map[string]interface{}{}
	}
	return cfg
}

func BuildIntegrationTargetConfig(targetKey, purpose string, enabled bool, allowedActions []string, schema map[string]interface{}, config map[string]interface{}) string {
	payload := integrationTargetConfig{
		TargetKey:      strings.TrimSpace(targetKey),
		Purpose:        strings.TrimSpace(purpose),
		Enabled:        &enabled,
		AllowedActions: allowedActions,
		Schema:         schema,
		Config:         config,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func validateAndNormalizeDecision(decision domain.ProcessDecision, targets []domain.ForwardTarget) (domain.ProcessDecision, error) {
	action := strings.TrimSpace(decision.ActionName)
	if action == "" {
		return decision, errors.New("missing action")
	}
	if _, ok := allowedActionFamilies[action]; !ok {
		return decision, fmt.Errorf("unknown action family: %s", action)
	}
	if action == "store_mysql" || action == "no_action" || action == "manual_review" {
		return decision, nil
	}
	if decision.Params == nil {
		decision.Params = map[string]interface{}{}
	}

	targetKey, _ := decision.Params["integration_target_key"].(string)
	targetKey = strings.TrimSpace(targetKey)
	if targetKey == "" && action == "forward_http" {
		return decision, nil
	}
	target, ok := resolveIntegrationTarget(targets, targetKey, action)
	if !ok {
		return decision, fmt.Errorf("integration target not found for action=%s key=%s", action, targetKey)
	}
	decision.Params["integration_target_key"] = target.TargetKey

	switch action {
	case "crm_upsert":
		if !hasMapParam(decision.Params, "entity_payload", "lead") {
			return decision, errors.New("crm_upsert requires entity_payload or lead params")
		}
	case "ticket_create":
		if !hasMapParam(decision.Params, "entity_payload", "ticket") {
			return decision, errors.New("ticket_create requires entity_payload or ticket params")
		}
	case "slack_notify":
		if !hasMapParam(decision.Params, "message_fields") && strings.TrimSpace(decision.ProcessedText) == "" {
			return decision, errors.New("slack_notify requires message_fields or processed_text")
		}
	}
	return decision, nil
}

func resolveIntegrationTarget(targets []domain.ForwardTarget, targetKey, action string) (domain.ForwardTarget, bool) {
	if targetKey != "" {
		for _, raw := range targets {
			target := HydrateForwardTarget(raw)
			if !target.Enabled {
				continue
			}
			if target.TargetKey != targetKey {
				continue
			}
			if targetSupportsAction(target, action) {
				return target, true
			}
		}
		return domain.ForwardTarget{}, false
	}
	for _, raw := range targets {
		target := HydrateForwardTarget(raw)
		if !target.Enabled {
			continue
		}
		if targetSupportsAction(target, action) {
			return target, true
		}
	}
	return domain.ForwardTarget{}, false
}

func targetSupportsAction(target domain.ForwardTarget, action string) bool {
	if strings.TrimSpace(target.AllowedActionsJSON) != "" {
		var allowed []string
		if err := json.Unmarshal([]byte(target.AllowedActionsJSON), &allowed); err == nil && len(allowed) > 0 {
			for _, item := range allowed {
				if strings.EqualFold(strings.TrimSpace(item), action) {
					return true
				}
			}
			return false
		}
	}
	switch target.TargetType {
	case "http":
		return action == "forward_http" || action == "crm_upsert" || action == "ticket_create" || action == "slack_notify"
	case "telegram":
		return action == "forward_telegram" || action == "slack_notify"
	default:
		return action == "forward_http"
	}
}

func hasMapParam(params map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if raw, ok := params[key]; ok {
			if _, ok := raw.(map[string]interface{}); ok {
				return true
			}
		}
	}
	return false
}
