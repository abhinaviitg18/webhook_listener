package mail

import (
	"context"
	"testing"

	"agenthook.store/internal/domain"
)

type stubReceiptGuardStore struct {
	identities map[string]domain.WebhookIdentity
}

func (s stubReceiptGuardStore) GetWebhookIdentityByLocalPart(_ context.Context, localPart string) (domain.WebhookIdentity, error) {
	if item, ok := s.identities[localPart]; ok {
		return item, nil
	}
	return domain.WebhookIdentity{}, domain.ErrWebhookIdentityNotFound
}

func TestReceiptGuardServiceStopsBlockedAndDeleted(t *testing.T) {
	service := &ReceiptGuardService{
		Store: stubReceiptGuardStore{
			identities: map[string]domain.WebhookIdentity{
				"owner.blocked": {LocalPart: "owner.blocked", Status: "blocked"},
				"owner.deleted": {LocalPart: "owner.deleted", Status: "deleted_tombstoned"},
				"owner.active":  {LocalPart: "owner.active", Status: "active"},
			},
		},
		MailDomain: "app.agenthook.store",
	}

	cases := []struct {
		address     string
		disposition string
	}{
		{"owner.blocked@app.agenthook.store", ReceiptDispositionStopRuleSet},
		{"owner.deleted@app.agenthook.store", ReceiptDispositionStopRuleSet},
		{"owner.active@app.agenthook.store", ReceiptDispositionContinue},
		{"unknown.anything@app.agenthook.store", ReceiptDispositionContinue},
		{"badaddress", ReceiptDispositionStopRuleSet},
	}

	for _, tc := range cases {
		result, err := service.EvaluateRecipient(context.Background(), tc.address)
		if err != nil {
			t.Fatalf("EvaluateRecipient(%q): %v", tc.address, err)
		}
		if result.Disposition != tc.disposition {
			t.Fatalf("EvaluateRecipient(%q) disposition=%s want=%s", tc.address, result.Disposition, tc.disposition)
		}
	}
}
