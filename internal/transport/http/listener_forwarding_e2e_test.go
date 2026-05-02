package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/service"
	"agenthook.store/internal/store"
)

type forwardingRouterLLM struct {
	targetKey   string
	sourceType  string
	skillKey    string
	processText string
}

func (f forwardingRouterLLM) SuggestAction(_ context.Context, typeKey, payload string, _ []domain.PineconeMemory, _ []string) (domain.ProcessDecision, error) {
	if strings.HasPrefix(typeKey, "route:") && strings.Contains(typeKey, f.sourceType) {
		return domain.ProcessDecision{
			ActionName: f.skillKey,
			Reason:     "router-forward",
			Params: map[string]interface{}{
				"spam_label":             "not_spam",
				"skill_candidates":       []string{f.skillKey},
				"candidate_action":       "forward_http",
				"integration_target_key": f.targetKey,
			},
		}, nil
	}
	if typeKey == f.sourceType {
		return domain.ProcessDecision{
			ActionName:    "forward_http",
			Reason:        "llm-forward",
			ProcessedText: f.processText,
			Params: map[string]interface{}{
				"integration_target_key": f.targetKey,
			},
		}, nil
	}
	return domain.ProcessDecision{ActionName: "store_mysql", Reason: "default", Params: map[string]interface{}{}}, nil
}

type captureTransport struct {
	base     http.RoundTripper
	requests []string
	statuses []int
	bodies   []string
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.base.RoundTrip(req)
	c.requests = append(c.requests, req.Method+" "+req.URL.String())
	if resp != nil {
		c.statuses = append(c.statuses, resp.StatusCode)
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.bodies = append(c.bodies, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return resp, err
}

func TestListenerToListenerForwardingDeterministicAPI(t *testing.T) {
	ts, token, _, capture := newForwardingTestServer(t, nil)
	sink := createListener(t, ts, token, map[string]any{
		"provider":          "generic-json",
		"listener_id":       "sinkdet",
		"deployment_mode":   "multitenant",
		"plain_text_action": "no_action",
		"use_llm_fallback":  false,
		"secret_value":      "sinkdetsecret",
	})
	targetKey := "relay_target_det"
	createForwardTarget(t, ts, token, map[string]any{
		"target_key":      targetKey,
		"target_type":     "http",
		"purpose":         "relay to sinkdet",
		"enabled":         true,
		"allowed_actions": []string{"forward_http"},
		"config": map[string]any{
			"url": sink["webhook_url"],
			"headers": map[string]any{
				"x-agenthook-source-listener": "source_det",
			},
		},
	})
	source := createListener(t, ts, token, map[string]any{
		"provider":          "generic-json",
		"listener_id":       "sourcedet",
		"deployment_mode":   "multitenant",
		"plain_text_action": "",
		"use_llm_fallback":  false,
		"secret_value":      "sourcedetsecret",
	})
	createSkill(t, ts, token, map[string]any{
		"type_key":       source["type_key"],
		"skill_key":      "det_forward_skill",
		"skill_prompt":   "",
		"match_contains": "deterministic-forward-e2e",
		"forced_action":  "forward_http",
		"priority":       1,
		"enabled":        true,
	})

	testRunID := fmt.Sprintf("det-%d", time.Now().UnixNano())
	payload := map[string]any{
		"test_run_id": testRunID,
		"message":     "deterministic-forward-e2e source payload",
	}
	resp := postWebhook(t, source["webhook_url"].(string), payload)
	sourceResponseEvent, _ := resp["event"].(map[string]any)
	if decision, ok := resp["decision"].(map[string]any); ok {
		if decision["action_name"] != "forward_http" {
			t.Fatalf("expected forward_http decision, got %v", decision["action_name"])
		}
	} else {
		t.Fatalf("missing decision in webhook response")
	}

	sourceEvents := listListenerEvents(t, ts, token, "sourcedet", "generic-json")
	sourceEvent := findEventByID(t, sourceEvents, fmt.Sprint(sourceResponseEvent["id"]))
	if sourceEvent["action_selected"] != "forward_http" {
		t.Fatalf("expected source action forward_http, got %v", sourceEvent["action_selected"])
	}
	if sourceEvent["status"] != "processed" {
		t.Fatalf("expected source status processed, got %v", sourceEvent["status"])
	}
	if len(capture.requests) == 0 {
		t.Fatalf("expected outgoing forward request")
	}
	if capture.statuses[len(capture.statuses)-1] != http.StatusAccepted {
		t.Fatalf("expected sink webhook to return 202, got requests=%v statuses=%v bodies=%v", capture.requests, capture.statuses, capture.bodies)
	}

	sinkEvents := listListenerEvents(t, ts, token, "sinkdet", "generic-json")
	if len(sinkEvents) == 0 {
		t.Fatalf("expected sink listener to receive forwarded event, got requests=%v statuses=%v bodies=%v", capture.requests, capture.statuses, capture.bodies)
	}
	sinkEvent := findEventByMarker(t, sinkEvents, testRunID)
	if sinkEvent["status"] != "processed" {
		t.Fatalf("expected sink status processed, got %v", sinkEvent["status"])
	}
	if sinkEvent["action_selected"] != "no_action" {
		t.Fatalf("expected sink action no_action, got %v", sinkEvent["action_selected"])
	}
	if fmt.Sprint(sinkEvent["event_id"]) == fmt.Sprint(sourceEvent["event_id"]) {
		t.Fatalf("expected sink event id to differ from source event id")
	}
	if !strings.Contains(stringifyAny(t, sinkEvent["raw_payload_json"]), testRunID) {
		t.Fatalf("expected sink raw payload to contain test marker")
	}
}

func TestListenerToListenerForwardingLLMAPI(t *testing.T) {
	var llm forwardingRouterLLM
	ts, token, _, capture := newForwardingTestServer(t, &llm)
	sink := createListener(t, ts, token, map[string]any{
		"provider":          "generic-json",
		"listener_id":       "sinkllm",
		"deployment_mode":   "multitenant",
		"plain_text_action": "no_action",
		"use_llm_fallback":  false,
		"secret_value":      "sinkllmsecret",
	})
	targetKey := "relay_target_llm"
	createForwardTarget(t, ts, token, map[string]any{
		"target_key":      targetKey,
		"target_type":     "http",
		"purpose":         "relay to sinkllm",
		"enabled":         true,
		"allowed_actions": []string{"forward_http"},
		"config": map[string]any{
			"url": sink["webhook_url"],
			"headers": map[string]any{
				"x-agenthook-source-listener": "source_llm",
			},
		},
	})
	source := createListener(t, ts, token, map[string]any{
		"provider":          "generic-json",
		"listener_id":       "sourcellm",
		"deployment_mode":   "multitenant",
		"plain_text_action": "",
		"use_llm_fallback":  true,
		"secret_value":      "sourcellmsecret",
	})
	llm = forwardingRouterLLM{
		targetKey:   targetKey,
		sourceType:  source["type_key"].(string),
		skillKey:    "llm_forward_skill",
		processText: "Forwarded by router/llm",
	}
	createSkill(t, ts, token, map[string]any{
		"type_key":       source["type_key"],
		"skill_key":      llm.skillKey,
		"skill_prompt":   "When this matches, choose candidate_action forward_http and integration_target_key " + targetKey,
		"match_contains": "llm-forward-e2e",
		"forced_action":  "",
		"priority":       1,
		"enabled":        true,
	})

	testRunID := fmt.Sprintf("llm-%d", time.Now().UnixNano())
	payload := map[string]any{
		"test_run_id": testRunID,
		"message":     "llm-forward-e2e source payload",
	}
	resp := postWebhook(t, source["webhook_url"].(string), payload)
	sourceResponseEvent, _ := resp["event"].(map[string]any)
	if decision, ok := resp["decision"].(map[string]any); ok {
		if decision["action_name"] != "forward_http" {
			t.Fatalf("expected forward_http decision, got %v", decision["action_name"])
		}
	} else {
		t.Fatalf("missing decision in webhook response")
	}

	sourceEvents := listListenerEvents(t, ts, token, "sourcellm", "generic-json")
	sourceEvent := findEventByID(t, sourceEvents, fmt.Sprint(sourceResponseEvent["id"]))
	if sourceEvent["action_selected"] != "forward_http" {
		t.Fatalf("expected source action forward_http, got %v", sourceEvent["action_selected"])
	}
	if sourceEvent["status"] != "processed" {
		t.Fatalf("expected source status processed, got %v", sourceEvent["status"])
	}
	if len(capture.requests) == 0 {
		t.Fatalf("expected outgoing forward request")
	}
	if capture.statuses[len(capture.statuses)-1] != http.StatusAccepted {
		t.Fatalf("expected sink webhook to return 202, got requests=%v statuses=%v bodies=%v", capture.requests, capture.statuses, capture.bodies)
	}

	sinkEvents := listListenerEvents(t, ts, token, "sinkllm", "generic-json")
	if len(sinkEvents) == 0 {
		t.Fatalf("expected sink listener to receive forwarded event, got requests=%v statuses=%v bodies=%v", capture.requests, capture.statuses, capture.bodies)
	}
	sinkEvent := findEventByMarker(t, sinkEvents, testRunID)
	if sinkEvent["status"] != "processed" {
		t.Fatalf("expected sink status processed, got %v", sinkEvent["status"])
	}
	if sinkEvent["action_selected"] != "no_action" {
		t.Fatalf("expected sink action no_action, got %v", sinkEvent["action_selected"])
	}
	if fmt.Sprint(sinkEvent["event_id"]) == fmt.Sprint(sourceEvent["event_id"]) {
		t.Fatalf("expected sink event id to differ from source event id")
	}
	if !strings.Contains(stringifyAny(t, sinkEvent["raw_payload_json"]), testRunID) {
		t.Fatalf("expected sink raw payload to contain test marker")
	}
}

func newForwardingTestServer(t *testing.T, llm domain.LLMClient) (*httptest.Server, string, map[string]interface{}, *captureTransport) {
	t.Helper()
	st := store.NewMemoryStore()
	actionSvc := service.NewActionService(nil)
	actionSvc.Store = st
	proc := &service.Processor{
		Store:    st,
		LLM:      llm,
		Executor: actionSvc,
	}
	h := &Handler{Store: st, Processor: proc}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	h.PublicBaseURL = ts.URL
	client := ts.Client()
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	capture := &captureTransport{base: base}
	actionSvc.Client = &http.Client{Transport: capture}

	acct, token, err := registerAccount(ts.URL, fmt.Sprintf("api-forward-%d@agentmail.to", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	return ts, token, acct, capture
}

func createListener(t *testing.T, ts *httptest.Server, token string, body map[string]any) map[string]any {
	t.Helper()
	return postAuthedJSON(t, ts.URL+"/v1/listeners", token, body, http.StatusCreated)
}

func createForwardTarget(t *testing.T, ts *httptest.Server, token string, body map[string]any) map[string]any {
	t.Helper()
	return postAuthedJSON(t, ts.URL+"/api/forward-targets", token, body, http.StatusCreated)
}

func createSkill(t *testing.T, ts *httptest.Server, token string, body map[string]any) map[string]any {
	t.Helper()
	return postAuthedJSON(t, ts.URL+"/api/policy/skills", token, body, http.StatusCreated)
}

func postAuthedJSON(t *testing.T, url, token string, body map[string]any, wantStatus int) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("unexpected status %d for %s", resp.StatusCode, url)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postWebhook(t *testing.T, webhookURL string, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected webhook status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listListenerEvents(t *testing.T, ts *httptest.Server, token, listenerID, provider string) []map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/listeners/"+listenerID+"/events?provider="+provider, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected events status %d", resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func findEventByMarker(t *testing.T, events []map[string]any, marker string) map[string]any {
	t.Helper()
	for _, item := range events {
		if strings.Contains(stringifyAny(t, item["raw_payload_json"]), marker) || strings.Contains(stringifyAny(t, item["payload_json"]), marker) || strings.Contains(stringifyAny(t, item["processed_text"]), marker) {
			return item
		}
	}
	b, _ := json.Marshal(events)
	t.Fatalf("could not find event containing marker %q in events=%s", marker, string(b))
	return nil
}

func findEventByID(t *testing.T, events []map[string]any, eventID string) map[string]any {
	t.Helper()
	for _, item := range events {
		if fmt.Sprint(item["event_id"]) == eventID {
			return item
		}
	}
	b, _ := json.Marshal(events)
	t.Fatalf("could not find event id %q in events=%s", eventID, string(b))
	return nil
}

func stringifyAny(t *testing.T, raw any) string {
	t.Helper()
	if raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		b, err := json.Marshal(typed)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}
