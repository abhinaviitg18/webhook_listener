package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"agenthook.store/internal/domain"
)

const (
	spamLabelSpam    = "spam"
	spamLabelNotSpam = "not_spam"
	spamLabelUnknown = "unknown"
)

type skillCandidate struct {
	Skill          domain.WebhookSkill
	MatchedTokens  []string
	Score          int
	HardLockAction bool
}

type routingDecision struct {
	SpamLabel            string
	Candidates           []skillCandidate
	SelectedSkillKeys    []string
	CandidateAction      string
	IntegrationTargetKey string
	UsedRouter           bool
}

func routeEvent(ctx context.Context, llm domain.LLMClient, typeKey, payload string, skills []domain.WebhookSkill) routingDecision {
	out := routingDecision{
		SpamLabel:  detectSpamLabel(payload),
		Candidates: rankSkillCandidates(skills, payload),
	}

	if out.SpamLabel == spamLabelSpam && len(out.Candidates) == 0 {
		return out
	}

	if len(out.Candidates) == 1 {
		out.SelectedSkillKeys = []string{out.Candidates[0].Skill.SkillKey}
		if out.Candidates[0].HardLockAction {
			out.CandidateAction = strings.TrimSpace(out.Candidates[0].Skill.ForcedAction)
		}
		return out
	}

	if llm == nil || len(out.Candidates) == 0 {
		for i, candidate := range out.Candidates {
			if i >= 3 {
				break
			}
			out.SelectedSkillKeys = append(out.SelectedSkillKeys, candidate.Skill.SkillKey)
		}
		return out
	}

	routerDecision, ok := routeWithLLM(ctx, llm, typeKey, payload, out.SpamLabel, out.Candidates)
	if !ok {
		for i, candidate := range out.Candidates {
			if i >= 3 {
				break
			}
			out.SelectedSkillKeys = append(out.SelectedSkillKeys, candidate.Skill.SkillKey)
		}
		return out
	}

	out.UsedRouter = true
	if routerDecision.SpamLabel != "" {
		out.SpamLabel = routerDecision.SpamLabel
	}
	out.SelectedSkillKeys = routerDecision.SelectedSkillKeys
	out.CandidateAction = routerDecision.CandidateAction
	out.IntegrationTargetKey = routerDecision.IntegrationTargetKey
	if len(out.SelectedSkillKeys) == 0 && len(out.Candidates) > 0 {
		out.SelectedSkillKeys = []string{out.Candidates[0].Skill.SkillKey}
	}
	return out
}

func rankSkillCandidates(skills []domain.WebhookSkill, payload string) []skillCandidate {
	if len(skills) == 0 {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(payload))
	var out []skillCandidate
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		tokens := matchedTokens(normalized, skill.MatchContains)
		if len(tokens) == 0 && strings.TrimSpace(skill.MatchContains) != "" {
			continue
		}
		score := len(tokens) * 10
		if strings.TrimSpace(skill.ForcedAction) != "" {
			score += 3
		}
		if strings.TrimSpace(skill.SkillPrompt) != "" {
			score += 2
		}
		if strings.Contains(strings.ToLower(skill.SkillKey), "spam") && detectSpamLabel(normalized) == spamLabelSpam {
			score += 20
		}
		out = append(out, skillCandidate{
			Skill:          skill,
			MatchedTokens:  tokens,
			Score:          score,
			HardLockAction: strings.TrimSpace(skill.ForcedAction) != "" && !strings.EqualFold(strings.TrimSpace(skill.ForcedAction), "store_mysql"),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			if out[i].Skill.Priority == out[j].Skill.Priority {
				return out[i].Skill.CreatedAt.Before(out[j].Skill.CreatedAt)
			}
			return out[i].Skill.Priority < out[j].Skill.Priority
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func matchedTokens(normalizedPayload, matchContains string) []string {
	var out []string
	for _, raw := range strings.Split(matchContains, ",") {
		token := strings.TrimSpace(strings.ToLower(raw))
		if token == "" {
			continue
		}
		if strings.Contains(normalizedPayload, token) {
			out = append(out, token)
		}
	}
	return out
}

func detectSpamLabel(payload string) string {
	normalized := strings.ToLower(strings.TrimSpace(payload))
	if normalized == "" {
		return spamLabelUnknown
	}
	spamSignals := []string{
		"unsubscribe", "limited time", "sale", "discount", "offer ends", "newsletter",
		"promotional", "marketing", "buy now", "click here", "special offer",
	}
	for _, signal := range spamSignals {
		if strings.Contains(normalized, signal) {
			return spamLabelSpam
		}
	}
	transactionalSignals := []string{
		"verification code", "invoice", "receipt", "workflow", "incident", "error", "support", "ticket", "lead", "demo_request",
	}
	for _, signal := range transactionalSignals {
		if strings.Contains(normalized, signal) {
			return spamLabelNotSpam
		}
	}
	return spamLabelUnknown
}

func routeWithLLM(ctx context.Context, llm domain.LLMClient, typeKey, payload, spamLabel string, candidates []skillCandidate) (routingDecision, bool) {
	candidateViews := make([]map[string]interface{}, 0, len(candidates))
	available := make([]string, 0, len(candidates)+3)
	seen := map[string]struct{}{}
	for i, candidate := range candidates {
		if i >= 6 {
			break
		}
		candidateViews = append(candidateViews, map[string]interface{}{
			"skill_key":      candidate.Skill.SkillKey,
			"priority":       candidate.Skill.Priority,
			"matched_tokens": candidate.MatchedTokens,
			"forced_action":  strings.TrimSpace(candidate.Skill.ForcedAction),
			"purpose":        clampText(strings.TrimSpace(candidate.Skill.SkillPrompt), 180),
		})
		if _, ok := seen[candidate.Skill.SkillKey]; !ok {
			available = append(available, candidate.Skill.SkillKey)
			seen[candidate.Skill.SkillKey] = struct{}{}
		}
	}
	available = append(available, spamLabelSpam, spamLabelNotSpam, spamLabelUnknown)
	routerPayload, _ := json.Marshal(map[string]interface{}{
		"mode":                 "route_only",
		"type_key":             typeKey,
		"spam_label_initial":   spamLabel,
		"instructions":         "Choose the most relevant skill keys for this event. Put spam/not_spam/unknown in params.spam_label. Put selected skill keys in params.skill_candidates. Optionally set params.candidate_action and params.integration_target_key. action_name may be a skill key or spam label.",
		"candidate_skills":     candidateViews,
		"payload":              payload,
		"max_skill_candidates": 3,
	})
	d, err := llm.SuggestAction(ctx, "route:"+typeKey, string(routerPayload), nil, available)
	if err != nil {
		return routingDecision{}, false
	}
	out := routingDecision{}
	if raw, ok := d.Params["spam_label"].(string); ok {
		out.SpamLabel = strings.TrimSpace(raw)
	}
	out.SelectedSkillKeys = append(out.SelectedSkillKeys, strings.TrimSpace(d.ActionName))
	if raw, ok := d.Params["skill_candidates"]; ok {
		out.SelectedSkillKeys = extractStringSlice(raw)
	}
	if raw, ok := d.Params["candidate_action"].(string); ok {
		out.CandidateAction = strings.TrimSpace(raw)
	}
	if raw, ok := d.Params["integration_target_key"].(string); ok {
		out.IntegrationTargetKey = strings.TrimSpace(raw)
	}
	out.SelectedSkillKeys = filterKnownSkills(out.SelectedSkillKeys, candidates)
	if out.SpamLabel == "" && isSpamLabel(d.ActionName) {
		out.SpamLabel = d.ActionName
	}
	return out, len(out.SelectedSkillKeys) > 0 || out.SpamLabel != ""
}

func isSpamLabel(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case spamLabelSpam, spamLabelNotSpam, spamLabelUnknown:
		return true
	default:
		return false
	}
}

func filterKnownSkills(keys []string, candidates []skillCandidate) []string {
	if len(keys) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, candidate := range candidates {
		allowed[candidate.Skill.SkillKey] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func extractStringSlice(raw interface{}) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func selectSkillsForExecution(skills []domain.WebhookSkill, selected []string) []domain.WebhookSkill {
	if len(selected) == 0 {
		return nil
	}
	want := map[string]struct{}{}
	for _, key := range selected {
		want[key] = struct{}{}
	}
	out := make([]domain.WebhookSkill, 0, len(selected))
	for _, skill := range skills {
		if _, ok := want[skill.SkillKey]; ok {
			out = append(out, skill)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

func deriveActionHints(selected []domain.WebhookSkill) (string, string) {
	for _, skill := range selected {
		action := strings.TrimSpace(skill.ForcedAction)
		if action != "" && !strings.EqualFold(action, "store_mysql") {
			return action, ""
		}
	}
	return "", ""
}

func routeLogSummary(route routingDecision) string {
	candidateKeys := make([]string, 0, len(route.Candidates))
	for _, candidate := range route.Candidates {
		candidateKeys = append(candidateKeys, candidate.Skill.SkillKey)
	}
	summary := map[string]interface{}{
		"spam_label":             route.SpamLabel,
		"candidate_skills":       candidateKeys,
		"selected_skills":        route.SelectedSkillKeys,
		"candidate_action":       route.CandidateAction,
		"integration_target_key": route.IntegrationTargetKey,
		"used_router":            route.UsedRouter,
	}
	b, _ := json.Marshal(summary)
	return string(b)
}

func newManualReviewDecision(reason string) domain.ProcessDecision {
	return domain.ProcessDecision{
		ActionName: "manual_review",
		Reason:     fmt.Sprintf("manual review: %s", strings.TrimSpace(reason)),
		Params:     map[string]interface{}{},
	}
}
