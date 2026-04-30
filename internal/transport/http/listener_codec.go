package httpapi

import (
	"strings"

	"agenthook.store/internal/webhookid"
)

const listenerTypePrefix = "lis::"

type listenerRef struct {
	Provider       string `json:"provider"`
	ListenerID     string `json:"listener_id"`
	DeploymentMode string `json:"deployment_mode"`
}

func buildListenerTypeKey(provider, listenerID, deploymentMode string) string {
	p := normalizeProvider(provider)
	l := normalizeListenerID(listenerID)
	m := normalizeDeploymentMode(deploymentMode)
	return listenerTypePrefix + p + "::" + l + "::" + m
}

func parseListenerTypeKey(typeKey string) (listenerRef, bool) {
	raw := strings.TrimSpace(typeKey)
	if !strings.HasPrefix(raw, listenerTypePrefix) {
		return listenerRef{}, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, listenerTypePrefix), "::")
	if len(parts) != 3 {
		return listenerRef{}, false
	}
	ref := listenerRef{
		Provider:       normalizeProvider(parts[0]),
		ListenerID:     normalizeListenerID(parts[1]),
		DeploymentMode: normalizeDeploymentMode(parts[2]),
	}
	if ref.Provider == "" || ref.ListenerID == "" {
		return listenerRef{}, false
	}
	return ref, true
}

func normalizeProvider(in string) string {
	p := strings.ToLower(strings.TrimSpace(in))
	p = strings.ReplaceAll(p, " ", "-")
	return p
}

func normalizeListenerID(in string) string {
	l := strings.ToLower(strings.TrimSpace(in))
	l = strings.ReplaceAll(l, " ", "-")
	return l
}

func normalizeDeploymentMode(in string) string {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "enterprise", "single_tenant":
		return "single_tenant"
	default:
		return "multitenant"
	}
}

func normalizePublicAlias(in string) string {
	return webhookid.NormalizePublicAlias(in)
}

func normalizeWebhookSecret(in string) string {
	return webhookid.NormalizeWebhookSecret(in)
}

func isValidWebhookSecret(secret string) bool {
	return webhookid.IsValidManualSecret(secret)
}
