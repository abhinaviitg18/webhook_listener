package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompactPayloadForLLMSkipsSmallPayload(t *testing.T) {
	payload := `{"id":"evt_1","message":"hello"}`
	got := compactPayloadForLLM(payload, LLMCompactionConfig{
		Enabled:        true,
		ThresholdBytes: 1024,
	})
	if got.WasCompacted {
		t.Fatalf("expected small payload to bypass compaction")
	}
	if got.CompactedPayload != payload {
		t.Fatalf("expected payload to remain unchanged")
	}
}

func TestCompactPayloadForLLMCompactsLargeNestedPayload(t *testing.T) {
	payload := `{"workflow":"deploy","repository":{"name":"webhook_listener","owner":{"avatar_url":"https://example.com/avatar.png","login":"agenthook"}},"jobs":[{"name":"build"},{"name":"test"},{"name":"deploy"}],"description":"` + strings.Repeat("x", 800) + `"}`
	got := compactPayloadForLLM(payload, LLMCompactionConfig{
		Enabled:         true,
		ThresholdBytes:  100,
		MaxStringBytes:  120,
		MaxArrayItems:   2,
		MaxObjectFields: 4,
	})
	if !got.WasCompacted {
		t.Fatalf("expected large payload to be compacted")
	}
	if got.CompactedBytes >= got.OriginalBytes {
		t.Fatalf("expected compacted payload to shrink, original=%d compacted=%d", got.OriginalBytes, got.CompactedBytes)
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(got.CompactedPayload), &root); err != nil {
		t.Fatalf("expected valid json output: %v", err)
	}
	if _, ok := root["_compaction"]; !ok {
		t.Fatalf("expected compaction metadata wrapper")
	}
}

func TestCompactPayloadForLLMTruncatesLongStringsAndArrays(t *testing.T) {
	payload := `{"message":"` + strings.Repeat("a", 500) + `","items":[1,2,3,4,5,6],"url":"https://example.com/` + strings.Repeat("b", 200) + `"}`
	got := compactPayloadForLLM(payload, LLMCompactionConfig{
		Enabled:         true,
		ThresholdBytes:  100,
		MaxStringBytes:  80,
		MaxArrayItems:   3,
		MaxObjectFields: 10,
	})
	if got.TruncatedStrings == 0 {
		t.Fatalf("expected string truncation")
	}
	if got.TruncatedArrays == 0 {
		t.Fatalf("expected array truncation")
	}
	if !strings.Contains(got.CompactedPayload, "_truncated_items") {
		t.Fatalf("expected truncated array marker in payload")
	}
}

func TestCompactPayloadForLLMReducesURLHeavyMetadata(t *testing.T) {
	payload := `{"avatar_url":"https://cdn.example.com/` + strings.Repeat("img", 200) + `","html_url":"https://github.com/example/repo","status":"success","repository":"webhook_listener","metadata":{"subscription_url":"https://api.example.com/subscriptions/123"}}`
	got := compactPayloadForLLM(payload, LLMCompactionConfig{
		Enabled:         true,
		ThresholdBytes:  100,
		MaxStringBytes:  60,
		MaxArrayItems:   4,
		MaxObjectFields: 3,
	})
	if !got.WasCompacted {
		t.Fatalf("expected url-heavy payload to be compacted")
	}
	if !strings.Contains(got.CompactedPayload, `"status":"success"`) {
		t.Fatalf("expected high-signal status field to be preserved")
	}
	if strings.Contains(got.CompactedPayload, strings.Repeat("img", 40)) {
		t.Fatalf("expected noisy url string to be shortened")
	}
}
