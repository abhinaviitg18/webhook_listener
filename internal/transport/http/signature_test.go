package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"hookweb.club/internal/auth"
	"hookweb.club/internal/integrations"
	"hookweb.club/internal/service"
	"hookweb.club/internal/store"
)

func TestReceiveWebhook_VerifiesHTCSignatureWhenPresent(t *testing.T) {
	st := store.NewMemoryStore()
	acct, token, _ := st.CreateAccount(t.Context(), "7204909316@agentmail.to")
	wt, _ := st.CreateWebhookType(t.Context(), acct.ID, "generic-json", "store_mysql", true)
	_, secretRaw, _ := st.CreateSecret(t.Context(), acct.ID, wt.ID)
	proc := &service.Processor{Store: st, Pinecone: integrations.NewPineconeClient("", "", "default"), LLM: integrations.NewLLMClient("", "", "", ""), Executor: service.NewActionService(nil)}
	h := &Handler{Store: st, Processor: proc, VerifyHTCSignature: true}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := []byte(`{"event_id":"evt_sig_1","event_type":"inbox.message.received"}`)
	tsRFC := time.Now().UTC().Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(secretRaw))
	mac.Write([]byte(strconv.FormatInt(parseRFC3339(tsRFC).Unix(), 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	goodSig := "v1=" + hex.EncodeToString(mac.Sum(nil))

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/url/"+acct.Slug+"/generic-json/"+secretRaw, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HTC-Webhook-Timestamp", tsRFC)
	req.Header.Set("X-HTC-Webhook-Signature", goodSig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected accepted with valid signature, got %d", resp.StatusCode)
	}

	reqBad, _ := http.NewRequest(http.MethodPost, ts.URL+"/url/"+acct.Slug+"/generic-json/"+secretRaw, bytes.NewReader(payload))
	reqBad.Header.Set("Content-Type", "application/json")
	reqBad.Header.Set("X-HTC-Webhook-Timestamp", tsRFC)
	reqBad.Header.Set("X-HTC-Webhook-Signature", "v1=deadbeef")
	respBad, err := http.DefaultClient.Do(reqBad)
	if err != nil {
		t.Fatal(err)
	}
	defer respBad.Body.Close()
	if respBad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized with bad signature, got %d", respBad.StatusCode)
	}

	reqList, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/events?limit=10", nil)
	reqList.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(reqList)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("events list status %d", listResp.StatusCode)
	}
}

func parseRFC3339(v string) time.Time {
	t, _ := time.Parse(time.RFC3339, v)
	return t
}
