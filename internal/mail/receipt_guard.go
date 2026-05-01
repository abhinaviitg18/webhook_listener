package mail

import (
	"context"
	"strings"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/webhookid"
)

const (
	ReceiptDispositionContinue    = "CONTINUE"
	ReceiptDispositionStopRuleSet = "STOP_RULE_SET"
)

type ReceiptGuardStore interface {
	GetWebhookIdentityByLocalPart(ctx context.Context, localPart string) (domain.WebhookIdentity, error)
}

type ReceiptGuardResult struct {
	Disposition string `json:"disposition"`
	Reason      string `json:"reason,omitempty"`
	Status      string `json:"status,omitempty"`
	LocalPart   string `json:"local_part,omitempty"`
}

type ReceiptGuardService struct {
	Store      ReceiptGuardStore
	MailDomain string
}

func (s *ReceiptGuardService) EvaluateRecipient(ctx context.Context, rawAddress string) (ReceiptGuardResult, error) {
	publicAlias, secret, domainName, ok := webhookid.ParseEmailAddress(rawAddress)
	if !ok {
		return ReceiptGuardResult{
			Disposition: ReceiptDispositionStopRuleSet,
			Reason:      "invalid_mailbox_address",
		}, nil
	}
	expectedDomain := strings.TrimSpace(strings.ToLower(s.MailDomain))
	if expectedDomain != "" && domainName != expectedDomain {
		return ReceiptGuardResult{
			Disposition: ReceiptDispositionStopRuleSet,
			Reason:      "unsupported_mail_domain",
		}, nil
	}
	localPart := webhookid.BuildLocalPart(publicAlias, secret)
	identity, err := s.Store.GetWebhookIdentityByLocalPart(ctx, localPart)
	if err != nil {
		if err == domain.ErrWebhookIdentityNotFound {
			return ReceiptGuardResult{
				Disposition: ReceiptDispositionContinue,
				Reason:      "unknown_identity",
				LocalPart:   localPart,
			}, nil
		}
		return ReceiptGuardResult{}, err
	}
	if identity.Status == "blocked" || identity.Status == "deleted_tombstoned" {
		return ReceiptGuardResult{
			Disposition: ReceiptDispositionStopRuleSet,
			Reason:      "identity_blocked",
			Status:      identity.Status,
			LocalPart:   localPart,
		}, nil
	}
	return ReceiptGuardResult{
		Disposition: ReceiptDispositionContinue,
		Reason:      "identity_active",
		Status:      identity.Status,
		LocalPart:   localPart,
	}, nil
}
