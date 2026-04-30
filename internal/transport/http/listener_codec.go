package httpapi

import (
	"regexp"
	"strings"
)

const listenerTypePrefix = "lis::"

var manualSecretPattern = regexp.MustCompile(`^[a-z0-9_-]{8,128}$`)

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
	alias := strings.ToLower(strings.TrimSpace(in))
	alias = strings.ReplaceAll(alias, " ", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range alias {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !valid {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
			continue
		}
		if r == '-' {
			if lastDash {
				continue
			}
			lastDash = true
		} else {
			lastDash = false
		}
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), "-_")
}

func normalizeWebhookSecret(in string) string {
	return strings.TrimSpace(in)
}

func isValidWebhookSecret(secret string) bool {
	return manualSecretPattern.MatchString(secret)
}
