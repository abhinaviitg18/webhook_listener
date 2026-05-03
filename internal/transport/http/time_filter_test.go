package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agenthook.store/internal/auth"
	"agenthook.store/internal/domain"
	"agenthook.store/internal/store"
)

func TestListEventsByTime(t *testing.T) {
	st := store.NewMemoryStore()
	h := &Handler{Store: st}
	r := NewRouter(h, auth.TokenVerifier{Store: st})
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Register and get token
	acct, token, err := st.CreateAccount(context.Background(), "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Create some events at different times
	now := time.Now().UTC()
	
	// Event 1: 10 minutes ago
	st.CreateEvent(context.Background(), domain.WebhookEvent{
		AccountID: acct.ID,
		TypeKey:   "test-10m",
		CreatedAt: now.Add(-10 * time.Minute),
	})
	
	// Event 2: 2 hours ago
	st.CreateEvent(context.Background(), domain.WebhookEvent{
		AccountID: acct.ID,
		TypeKey:   "test-2h",
		CreatedAt: now.Add(-2 * time.Hour),
	})

	// Event 3: 3 days ago
	st.CreateEvent(context.Background(), domain.WebhookEvent{
		AccountID: acct.ID,
		TypeKey:   "test-3d",
		CreatedAt: now.Add(-3 * 24 * time.Hour),
	})

	tests := []struct {
		window string
		count  int
	}{
		{"15m", 1},
		{"3h", 2},
		{"4d", 3},
		{"5m", 0},
	}

	for _, tt := range tests {
		t.Run(tt.window, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/events/by-time?window=%s", ts.URL, tt.window), nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}

			var events []domain.WebhookEvent
			_ = json.NewDecoder(resp.Body).Decode(&events)

			if len(events) != tt.count {
				t.Errorf("expected %d events, got %d", tt.count, len(events))
			}
		})
	}
}
