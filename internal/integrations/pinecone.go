package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agenthook.store/internal/domain"
)

type PineconeClient struct {
	Enabled   bool
	APIKey    string
	IndexURL  string
	Namespace string
	Client    *http.Client
}

type recordsSearchResp struct {
	Result struct {
		Hits []struct {
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Fields map[string]interface{} `json:"fields"`
		} `json:"hits"`
	} `json:"result"`
}

func NewPineconeClient(enabled bool, apiKey, indexURL, namespace string) *PineconeClient {
	return &PineconeClient{
		Enabled:   enabled,
		APIKey:    strings.TrimSpace(apiKey),
		IndexURL:  strings.TrimSpace(indexURL),
		Namespace: strings.TrimSpace(namespace),
		Client:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *PineconeClient) Query(ctx context.Context, accountID, query string, topK int) ([]domain.PineconeMemory, error) {
	if !p.Enabled || p.APIKey == "" || p.IndexURL == "" || strings.TrimSpace(query) == "" {
		return []domain.PineconeMemory{}, nil
	}
	if topK <= 0 {
		topK = 5
	}
	body := map[string]interface{}{
		"query": map[string]interface{}{
			"inputs": map[string]interface{}{"text": query},
			"top_k":  topK,
			"filter": map[string]interface{}{"account_id": accountID},
		},
		"fields": []string{"account_id", "type_key", "summary", "chunk_text", "event_id", "updated_at"},
	}
	b, _ := json.Marshal(body)
	url := strings.TrimRight(p.IndexURL, "/") + "/records/namespaces/" + p.Namespace + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pinecone-Api-Version", "2026-04")
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pinecone records search failed: %s", resp.Status)
	}
	var out recordsSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	result := make([]domain.PineconeMemory, 0, len(out.Result.Hits))
	for _, h := range out.Result.Hits {
		summary, _ := h.Fields["summary"].(string)
		if summary == "" {
			summary, _ = h.Fields["chunk_text"].(string)
		}
		result = append(result, domain.PineconeMemory{ID: h.ID, Score: h.Score, Summary: summary, Meta: h.Fields})
	}
	return result, nil
}

func (p *PineconeClient) UpsertOrUpdate(ctx context.Context, accountID, typeKey, eventID, canonicalPayload string, prior []domain.PineconeMemory) error {
	if !p.Enabled || p.APIKey == "" || p.IndexURL == "" || strings.TrimSpace(canonicalPayload) == "" {
		return nil
	}
	recordID := "rec-" + strings.ReplaceAll(eventID, "-", "")
	if len(prior) > 0 {
		best := prior[0]
		metaType, _ := best.Meta["type_key"].(string)
		if best.Score >= 0.93 && strings.EqualFold(strings.TrimSpace(metaType), strings.TrimSpace(typeKey)) {
			recordID = best.ID
		}
	}
	record := map[string]interface{}{
		"_id":         recordID,
		"chunk_text":  canonicalPayload,
		"summary":     canonicalPayload,
		"account_id":  accountID,
		"type_key":    typeKey,
		"event_id":    eventID,
		"updated_at":  time.Now().UTC().Format(time.RFC3339),
		"source":      "agenthook-auto",
	}
	line, _ := json.Marshal(record)
	url := strings.TrimRight(p.IndexURL, "/") + "/records/namespaces/" + p.Namespace + "/upsert"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(append(line, '\n')))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", p.APIKey)
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("X-Pinecone-Api-Version", "2026-04")
	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pinecone upsert failed: %s", resp.Status)
	}
	return nil
}
