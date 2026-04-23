package service

import (
	"context"
	"encoding/json"
	"testing"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/store"
)

type fakeClassifier struct{ out domain.TypeResolution }

func (f fakeClassifier) ClassifyType(_ context.Context, _ string, _ map[string]string, _ map[string]interface{}) (domain.TypeResolution, error) {
	return f.out, nil
}

func TestTypeResolver_DeterministicMultipleStructures(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")

	sigs := []domain.WebhookTypeSignature{
		{AccountID: acct.ID, TypeKey: "github-push", Enabled: true, ConfidenceThreshold: 0.7, RequiredKeysJSON: mustJSON([]string{"$.repository.full_name", "$.head_commit.id"}), ShapeHintsJSON: mustJSON(map[string]string{"$.repository": "object", "$.head_commit": "object"}), HeaderHintsJSON: `{}`},
		{AccountID: acct.ID, TypeKey: "stripe-payment", Enabled: true, ConfidenceThreshold: 0.7, RequiredKeysJSON: mustJSON([]string{"$.type", "$.data.object.amount"}), ShapeHintsJSON: mustJSON(map[string]string{"$.data": "object"}), HeaderHintsJSON: `{}`},
		{AccountID: acct.ID, TypeKey: "slack-event", Enabled: true, ConfidenceThreshold: 0.7, RequiredKeysJSON: mustJSON([]string{"$.event.type", "$.team_id"}), ShapeHintsJSON: mustJSON(map[string]string{"$.event": "object"}), HeaderHintsJSON: `{}`},
	}
	for _, s := range sigs {
		if _, err := st.CreateTypeSignature(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}

	r := NewTypeResolver(st, fakeClassifier{}, fakeClassifier{})

	cases := []struct {
		name     string
		payload  string
		expected string
	}{
		{"github", `{"repository":{"full_name":"a/b"},"head_commit":{"id":"abc"}}`, "github-push"},
		{"stripe", `{"type":"payment_intent.succeeded","data":{"object":{"amount":3000}}}`, "stripe-payment"},
		{"slack", `{"team_id":"T123","event":{"type":"message","text":"hi"}}`, "slack-event"},
	}
	for _, tc := range cases {
		res, err := r.Resolve(context.Background(), acct.ID, tc.payload, map[string]string{})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.TypeKey != tc.expected || res.Source != "deterministic" {
			t.Fatalf("%s: expected %s deterministic, got %+v", tc.name, tc.expected, res)
		}
	}
}

func TestTypeResolver_LLMDisagreementPicksBest(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	r := NewTypeResolver(st,
		fakeClassifier{out: domain.TypeResolution{TypeKey: "github-push", Confidence: 0.91}},
		fakeClassifier{out: domain.TypeResolution{TypeKey: "stripe-payment", Confidence: 0.83}},
	)
	res, err := r.Resolve(context.Background(), acct.ID, `{"x":1}`, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ManualReview || res.Source != "llm_disagreement_resolved" || res.TypeKey != "github-push" {
		t.Fatalf("expected resolved disagreement choosing best candidate, got %+v", res)
	}
}

func TestApplyDSLExtraction(t *testing.T) {
	payload := `{"type":"payment_intent.succeeded","data":{"object":{"amount":3000,"currency":"usd"}}}`
	dsl := `{"extract":{"event":"$.type","amount":"$.data.object.amount","currency":"$.data.object.currency"},"constants":{"provider":"stripe"}}`
	out, err := applyDSL(payload, dsl)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["provider"] != "stripe" || got["event"] != "payment_intent.succeeded" {
		t.Fatalf("unexpected transformed payload: %v", got)
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestTypeResolver_AutoPromotesValidatedShadowActive(t *testing.T) {
	st := store.NewMemoryStore()
	acct, _, _ := st.CreateAccount(context.Background(), "7204909316@agentmail.to")
	resolution := domain.TypeResolution{
		TypeKey:    "new-unknown-type",
		Confidence: 0.95,
		TransformTemplate: map[string]interface{}{
			"extract": map[string]interface{}{
				"event_id": "$.event_id",
				"kind":     "$.kind",
			},
		},
	}
	r := NewTypeResolver(st, fakeClassifier{out: resolution}, fakeClassifier{out: resolution})
	r.AutoPromoter = NewAutoPromoter(st, AutoPromoteConfig{
		Enabled:           true,
		MinConfidence:     0.9,
		ValidatedToShadow: 1,
		ShadowToActive:    2,
		MinSuccessRate:    1.0,
	})

	// First event: LLM candidate discovered and immediately promoted to shadow.
	_, err := r.Resolve(context.Background(), acct.ID, `{"event_id":"evt1","kind":"alpha","payload":{"x":1}}`, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	st1, err := st.GetAutoPromoteState(context.Background(), acct.ID, "new-unknown-type")
	if err != nil {
		t.Fatal(err)
	}
	if st1.Status != "shadow" {
		t.Fatalf("expected shadow after first high-confidence candidate, got %+v", st1)
	}
	shadowTransform, err := st.GetLatestTransformByStatus(context.Background(), acct.ID, "new-unknown-type", "shadow")
	if err != nil {
		t.Fatal(err)
	}
	if shadowTransform.Engine != "dsl" {
		t.Fatalf("expected dsl transform in shadow, got %+v", shadowTransform)
	}

	// Second and third event: deterministic shadow traffic promotes to active.
	_, err = r.Resolve(context.Background(), acct.ID, `{"event_id":"evt2","kind":"alpha","payload":{"x":2}}`, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Resolve(context.Background(), acct.ID, `{"event_id":"evt3","kind":"alpha","payload":{"x":3}}`, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	st2, err := st.GetAutoPromoteState(context.Background(), acct.ID, "new-unknown-type")
	if err != nil {
		t.Fatal(err)
	}
	if st2.Status != "active" {
		t.Fatalf("expected active after shadow samples, got %+v", st2)
	}
	activeTransform, err := st.GetActiveTransform(context.Background(), acct.ID, "new-unknown-type")
	if err != nil {
		t.Fatal(err)
	}
	if activeTransform.Version < 1 {
		t.Fatalf("expected active transform version >=1, got %+v", activeTransform)
	}
}
