package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"agenthook.store/internal/domain"
)

type LLMClient struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
	Client   *http.Client
}

type chatReq struct {
	Model    string    `json:"model"`
	Messages []chatMsg `json:"messages"`
	Response any       `json:"response_format,omitempty"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message chatMsg `json:"message"`
	} `json:"choices"`
}

func NewLLMClient(provider, key, baseURL, model string) *LLMClient {
	normalizedProvider := strings.TrimSpace(strings.ToLower(provider))
	normalizedModel := normalizeModelAlias(normalizedProvider, strings.TrimSpace(model))
	return &LLMClient{
		Provider: normalizedProvider,
		APIKey:   strings.TrimSpace(key),
		BaseURL:  strings.TrimSpace(baseURL),
		Model:    normalizedModel,
		Client:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (l *LLMClient) SuggestAction(ctx context.Context, typeKey, payload string, memories []domain.PineconeMemory, available []string) (domain.ProcessDecision, error) {
	if l.APIKey == "" || l.BaseURL == "" {
		return domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm not configured", Params: map[string]interface{}{}}, nil
	}
	prompt := buildPrompt(typeKey, payload, memories, available)
	log.Printf("llm.suggest start provider=%s model=%s type_key=%s payload_bytes=%d prompt_bytes=%d memories=%d available_actions=%d", l.Provider, l.Model, typeKey, len(payload), len(prompt), len(memories), len(available))
	r := chatReq{
		Model:    l.Model,
		Messages: []chatMsg{{Role: "system", Content: "Return strict JSON: {\"action_name\": string, \"reason\": string, \"params\": object, \"processed_text\": string, \"tags\": [string]}. The \"processed_text\" should be a concise, human-readable summary of the webhook event based on the user's intent or policy. The \"tags\" array should contain relevant category labels for this message such as: marketing, promotion, newsletter, otp, personal, transactional, alert, system, broadcast, notification, or any other relevant domain-specific tags. Always include at least one tag. Optional params.memory_write_mode must be one of: update_or_insert, insert_only, none."}, {Role: "user", Content: prompt}},
	}
	b, _ := json.Marshal(r)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(l.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return domain.ProcessDecision{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.APIKey)
	resp, err := l.Client.Do(req)
	if err != nil {
		return domain.ProcessDecision{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 400 {
			snippet = snippet[:400]
		}
		log.Printf("llm.suggest failure provider=%s model=%s type_key=%s status=%s response_body=%q", l.Provider, l.Model, typeKey, resp.Status, snippet)
		return domain.ProcessDecision{}, fmt.Errorf("llm request failed: %s", resp.Status)
	}
	var parsed chatResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		log.Printf("llm.suggest decode_error provider=%s model=%s type_key=%s err=%v", l.Provider, l.Model, typeKey, err)
		return domain.ProcessDecision{}, err
	}
	if len(parsed.Choices) == 0 {
		log.Printf("llm.suggest empty_choices provider=%s model=%s type_key=%s", l.Provider, l.Model, typeKey)
		return domain.ProcessDecision{}, fmt.Errorf("empty llm response")
	}
	content := parsed.Choices[0].Message.Content
	log.Printf("llm.suggest success provider=%s model=%s type_key=%s response_bytes=%d", l.Provider, l.Model, typeKey, len(content))

	var d domain.ProcessDecision
	jsonContent := normalizeJSONResponse(content)
	if err := json.Unmarshal([]byte(jsonContent), &d); err != nil {
		log.Printf("llm.suggest parse_fallback provider=%s model=%s type_key=%s err=%v", l.Provider, l.Model, typeKey, err)
		return domain.ProcessDecision{ActionName: "store_mysql", Reason: "llm parse fallback", Params: map[string]interface{}{}}, nil
	}
	if d.ActionName == "" {
		d.ActionName = "store_mysql"
		d.Reason = "llm empty action fallback"
	}
	if d.Params == nil {
		d.Params = map[string]interface{}{}
	}
	log.Printf("llm.suggest parsed provider=%s model=%s type_key=%s action=%s tags=%d processed_text_bytes=%d", l.Provider, l.Model, typeKey, d.ActionName, len(d.Tags), len(d.ProcessedText))
	return d, nil
}

func buildPrompt(typeKey, payload string, memories []domain.PineconeMemory, available []string) string {
	return fmt.Sprintf(
		"Type: %s\nPayload: %s\nRelevant context: %s\nAvailable actions: %s\nPick the best action and params. Include params.memory_write_mode if memory behavior should be overridden.",
		typeKey,
		payload,
		compactMemories(memories),
		strings.Join(available, ","),
	)
}

func normalizeModelAlias(provider, model string) string {
	normalizedProvider := strings.TrimSpace(strings.ToLower(provider))
	normalizedModel := strings.TrimSpace(model)
	if normalizedProvider != "groq" {
		return normalizedModel
	}
	switch normalizedModel {
	case "llama3-70b-8192":
		return "llama-3.3-70b-versatile"
	case "llama3-8b-8192":
		return "llama-3.1-8b-instant"
	default:
		return normalizedModel
	}
}

func normalizeJSONResponse(content string) string {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```JSON")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end >= start {
		return trimmed[start : end+1]
	}
	return trimmed
}

func compactMemories(memories []domain.PineconeMemory) string {
	if len(memories) == 0 {
		return ""
	}
	const (
		maxMemories     = 3
		maxSummaryBytes = 220
		maxTotalBytes   = 800
	)
	parts := make([]string, 0, minInt(len(memories), maxMemories))
	total := 0
	for i, memory := range memories {
		if i >= maxMemories {
			break
		}
		summary := strings.TrimSpace(memory.Summary)
		if len(summary) > maxSummaryBytes {
			summary = summary[:maxSummaryBytes]
		}
		if summary == "" {
			continue
		}
		next := summary
		if len(parts) > 0 {
			next = " | " + next
		}
		if total+len(next) > maxTotalBytes {
			remaining := maxTotalBytes - total
			if remaining <= 0 {
				break
			}
			next = next[:remaining]
		}
		parts = append(parts, strings.TrimPrefix(next, " | "))
		total += len(next)
		if total >= maxTotalBytes {
			break
		}
	}
	return strings.Join(parts, " | ")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
