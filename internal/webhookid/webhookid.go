package webhookid

import "strings"

func NormalizePublicAlias(in string) string {
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

func NormalizeWebhookSecret(in string) string {
	return strings.TrimSpace(in)
}

func IsValidManualSecret(secret string) bool {
	secret = NormalizeWebhookSecret(secret)
	if secret == "" {
		return false
	}
	return !strings.ContainsAny(secret, "/?.#@")
}

func BuildEmailAddress(publicAlias, secret, domain string) string {
	return NormalizePublicAlias(publicAlias) + "." + NormalizeWebhookSecret(secret) + "@" + strings.TrimSpace(strings.ToLower(domain))
}

func ParseEmailAddress(address string) (publicAlias, secret, domain string, ok bool) {
	trimmed := strings.TrimSpace(strings.ToLower(address))
	localPart, host, found := strings.Cut(trimmed, "@")
	if !found || localPart == "" || host == "" {
		return "", "", "", false
	}
	alias, sec, ok := ParseLocalPart(localPart)
	if !ok {
		return "", "", "", false
	}
	return alias, sec, host, true
}

func ParseLocalPart(localPart string) (publicAlias, secret string, ok bool) {
	localPart = strings.TrimSpace(localPart)
	idx := strings.Index(localPart, ".")
	if idx <= 0 || idx >= len(localPart)-1 {
		return "", "", false
	}
	return NormalizePublicAlias(localPart[:idx]), NormalizeWebhookSecret(localPart[idx+1:]), true
}
