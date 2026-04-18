package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"hookweb.club/internal/domain"
)

type ProviderTypeClassifier struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Client   *http.Client
}

type modelListResp struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func NewProviderTypeClassifier(provider, baseURL, apiKey, model string) *ProviderTypeClassifier {
	return &ProviderTypeClassifier{Provider: provider, BaseURL: strings.TrimRight(baseURL, "/"), APIKey: strings.TrimSpace(apiKey), Model: strings.TrimSpace(model), Client: &http.Client{Timeout: 8 * time.Second}}
}

func (c *ProviderTypeClassifier) ClassifyType(ctx context.Context, payload string, headers map[string]string, summary map[string]interface{}) (domain.TypeResolution, error) {
	if c.APIKey == "" || c.BaseURL == "" {
		return domain.TypeResolution{}, fmt.Errorf("%s classifier not configured", c.Provider)
	}
	model := c.Model
	if model == "" {
		m, err := c.pickModel(ctx)
		if err != nil {
			return domain.TypeResolution{}, err
		}
		model = m
	}
	prompt := buildTypePrompt(payload, headers, summary)
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "Return strict JSON only: {\"type_key\":string,\"confidence\":number,\"extract_fields\":object,\"transform_template\":object,\"reason\":string}"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return domain.TypeResolution{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return domain.TypeResolution{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return domain.TypeResolution{}, fmt.Errorf("%s classify failed: %s", c.Provider, resp.Status)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domain.TypeResolution{}, err
	}
	if len(parsed.Choices) == 0 {
		return domain.TypeResolution{}, fmt.Errorf("%s returned no choices", c.Provider)
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if !json.Valid([]byte(content)) {
		if extracted := extractJSONBlock(content); extracted != "" {
			content = extracted
		}
	}
	var out struct {
		TypeKey           string                 `json:"type_key"`
		Confidence        float64                `json:"confidence"`
		ExtractFields     map[string]string      `json:"extract_fields"`
		TransformTemplate map[string]interface{} `json:"transform_template"`
		Reason            string                 `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return domain.TypeResolution{}, err
	}
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return domain.TypeResolution{TypeKey: out.TypeKey, Confidence: out.Confidence, Source: c.Provider, Reason: out.Reason, ExtractFields: out.ExtractFields, TransformTemplateEngine: "dsl", TransformTemplate: out.TransformTemplate}, nil
}

func (c *ProviderTypeClassifier) pickModel(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("model discovery failed: %s", resp.Status)
	}
	var out modelListResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("no models returned for %s", c.Provider)
	}
	ids := []string{}
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	// Prefer instruction/foundation models, avoid safeguard-only models for classification.
	for _, id := range ids {
		lc := strings.ToLower(id)
		if strings.Contains(lc, "safeguard") {
			continue
		}
		if strings.Contains(lc, "llama") || strings.Contains(lc, "qwen") || strings.Contains(lc, "glm") || strings.Contains(lc, "gpt-oss-20b") || strings.Contains(lc, "gpt-oss-120b") {
			return id, nil
		}
	}
	for _, id := range ids {
		lc := strings.ToLower(id)
		if strings.Contains(lc, "instruct") || strings.Contains(lc, "oss") || strings.Contains(lc, "llama") || strings.Contains(lc, "qwen") || strings.Contains(lc, "glm") {
			return id, nil
		}
	}
	return ids[0], nil
}

func buildTypePrompt(payload string, headers map[string]string, summary map[string]interface{}) string {
	h, _ := json.Marshal(headers)
	s, _ := json.Marshal(summary)
	return "Classify webhook payload into a stable type key and generate deterministic extract mapping. Payload=" + payload + "\nHeaders=" + string(h) + "\nSummary=" + string(s) + "\nReturn transform_template as DSL JSON with extract paths."
}

func extractJSONBlock(in string) string {
	start := strings.Index(in, "{")
	end := strings.LastIndex(in, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	cand := strings.TrimSpace(in[start : end+1])
	if json.Valid([]byte(cand)) {
		return cand
	}
	return ""
}
