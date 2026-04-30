package service

import (
	"encoding/json"
	"html"
	"regexp"
	"sort"
	"strings"
)

var (
	htmlBlockBreakRe = regexp.MustCompile(`(?is)<\s*(br|/p|/div|/li|/tr|/h[1-6])\b[^>]*>`)
	htmlTagRe        = regexp.MustCompile(`(?is)<[^>]+>`)
	htmlCommentRe    = regexp.MustCompile(`(?is)<!--.*?-->`)
	scriptRe         = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleRe          = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	whitespaceRe     = regexp.MustCompile(`\s+`)
)

func sanitizePayloadForProcessing(payload string) string {
	var root interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return sanitizeHTMLText(payload)
	}
	sanitized := sanitizeJSONValue(root)
	b, err := json.Marshal(sanitized)
	if err != nil {
		return sanitizeHTMLText(payload)
	}
	return string(b)
}

func SanitizeHTMLText(input string) string {
	return sanitizeHTMLText(input)
}

func payloadToText(payload string) string {
	var root interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return clampText(sanitizeHTMLText(payload), 1200)
	}
	sanitized := sanitizeJSONValue(root)
	fragments := collectReadableText("", sanitized, nil)
	if len(fragments) == 0 {
		b, err := json.Marshal(sanitized)
		if err != nil {
			return clampText(sanitizeHTMLText(payload), 1200)
		}
		return clampText(string(b), 1200)
	}
	return clampText(strings.Join(fragments, "\n"), 1200)
}

func sanitizeJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			out[k] = sanitizeJSONValue(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeJSONValue(item))
		}
		return out
	case string:
		return sanitizeHTMLText(typed)
	default:
		return value
	}
}

func collectReadableText(key string, value interface{}, out []string) []string {
	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = collectReadableText(k, typed[k], out)
		}
	case []interface{}:
		for _, item := range typed {
			out = collectReadableText(key, item, out)
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || len(text) < 2 {
			return out
		}
		if !isTextLikeKey(key) && (looksLikeURL(text) || len(text) > 4000) {
			return out
		}
		if key != "" && isTextLikeKey(key) {
			out = append(out, key+": "+text)
			return out
		}
		if !looksLikeURL(text) {
			out = append(out, text)
		}
	}
	return out
}

func sanitizeHTMLText(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	normalized := scriptRe.ReplaceAllString(trimmed, " ")
	normalized = styleRe.ReplaceAllString(normalized, " ")
	normalized = htmlCommentRe.ReplaceAllString(normalized, " ")
	normalized = htmlBlockBreakRe.ReplaceAllString(normalized, "\n")
	normalized = htmlTagRe.ReplaceAllString(normalized, " ")
	normalized = html.UnescapeString(normalized)
	normalized = strings.ReplaceAll(normalized, "\u00a0", " ")
	normalized = whitespaceRe.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

func isTextLikeKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	for _, hint := range []string{"subject", "text", "body", "content", "message", "snippet", "preview", "summary", "description", "plain", "html", "title", "name", "from", "to", "cc", "bcc", "sender"} {
		if k == hint || strings.Contains(k, hint) {
			return true
		}
	}
	return false
}

func looksLikeURL(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}

func clampText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
