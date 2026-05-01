package store

import (
	"context"
	"errors"
	"testing"

	"agenthook.store/internal/domain"
)

func TestMemoryStore_RecreateDeletedIdentitySameOwner(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	acct, _, err := st.CreateAccount(ctx, "owner@example.com")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	wt, err := st.CreateWebhookType(ctx, acct.ID, "lis::generic-json::sameowner::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType: %v", err)
	}
	secret, err := st.CreateSecretWithValue(ctx, acct.ID, wt.ID, "restoreme")
	if err != nil {
		t.Fatalf("CreateSecretWithValue: %v", err)
	}
	if err := st.DeleteSecret(ctx, acct.ID, secret.ID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	restored, err := st.CreateSecretWithValue(ctx, acct.ID, wt.ID, "restoreme")
	if err != nil {
		t.Fatalf("same-owner recreate should succeed: %v", err)
	}
	if restored.SecretValue != "restoreme" {
		t.Fatalf("expected restored secret value, got %q", restored.SecretValue)
	}
}

func TestMemoryStore_TombstonedIdentityReservedAcrossAccounts(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	acct1, _, err := st.CreateAccount(ctx, "first@example.com")
	if err != nil {
		t.Fatalf("CreateAccount acct1: %v", err)
	}
	wt1, err := st.CreateWebhookType(ctx, acct1.ID, "lis::generic-json::reserved::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType acct1: %v", err)
	}
	if _, err := st.CreateSecretWithValue(ctx, acct1.ID, wt1.ID, "lockme"); err != nil {
		t.Fatalf("CreateSecretWithValue acct1: %v", err)
	}
	if _, err := st.UpdateAccountPublicAlias(ctx, acct1.ID, "first-team"); err != nil {
		t.Fatalf("UpdateAccountPublicAlias acct1: %v", err)
	}

	acct2, _, err := st.CreateAccount(ctx, "second@example.com")
	if err != nil {
		t.Fatalf("CreateAccount acct2: %v", err)
	}
	if _, err := st.UpdateAccountPublicAlias(ctx, acct2.ID, acct1.Slug); err != nil {
		t.Fatalf("UpdateAccountPublicAlias acct2: %v", err)
	}
	wt2, err := st.CreateWebhookType(ctx, acct2.ID, "lis::generic-json::reserved2::multitenant", "store_mysql", true)
	if err != nil {
		t.Fatalf("CreateWebhookType acct2: %v", err)
	}
	_, err = st.CreateSecretWithValue(ctx, acct2.ID, wt2.ID, "lockme")
	if !errors.Is(err, domain.ErrWebhookIdentityReserved) {
		t.Fatalf("expected ErrWebhookIdentityReserved, got %v", err)
	}
}
