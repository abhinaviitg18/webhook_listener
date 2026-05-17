package store

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/security"
	"agenthook.store/internal/webhookid"
)

type MySQLStore struct {
	db                           *sql.DB
	hasRawPayloadJSON            bool
	hasProcessedText             bool
	schemaCheckOnce              sync.Once
	eventSchemaMu                sync.Mutex
	eventSchemaReady             bool
	integrationSecretSchemaMu    sync.Mutex
	integrationSecretSchemaReady bool
	accountAliasSchemaMu         sync.Mutex
	accountAliasSchemaReady      bool
	webhookTypeSchemaMu          sync.Mutex
	webhookTypeSchemaReady       bool
	webhookIdentitySchemaMu      sync.Mutex
	webhookIdentitySchemaReady   bool
}

type eventSelectVariant struct {
	includeRaw       bool
	includeProcessed bool
	includeTags      bool
}

func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", NormalizeMySQLDSN(dsn))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	return &MySQLStore{db: db}, nil
}

func (s *MySQLStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *MySQLStore) columnExists(tableName, columnName string) bool {
	var exists int
	err := s.db.QueryRow(`
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		tableName, columnName,
	).Scan(&exists)
	return err == nil && exists > 0
}

func (s *MySQLStore) indexExists(tableName, indexName string) bool {
	var exists int
	err := s.db.QueryRow(`
SELECT COUNT(*)
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		tableName, indexName,
	).Scan(&exists)
	return err == nil && exists > 0
}

func (s *MySQLStore) ensureEventSchemaCapabilities() {
	s.schemaCheckOnce.Do(func() {
		s.hasRawPayloadJSON = s.columnExists("webhook_events", "raw_payload_json")
		s.hasProcessedText = s.columnExists("webhook_events", "processed_text")
		log.Printf("mysql.event_schema_capabilities has_raw_payload_json=%t has_processed_text=%t", s.hasRawPayloadJSON, s.hasProcessedText)
	})
}

func (s *MySQLStore) ensureAccountAliasSchema(ctx context.Context) error {
	s.accountAliasSchemaMu.Lock()
	defer s.accountAliasSchemaMu.Unlock()
	if s.accountAliasSchemaReady {
		return nil
	}
	if !s.columnExists("accounts", "public_alias") {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE accounts ADD COLUMN public_alias VARCHAR(128) NULL AFTER slug`); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE accounts SET public_alias=slug WHERE public_alias IS NULL OR public_alias=''`); err != nil {
		return err
	}
	if !s.indexExists("accounts", "uq_accounts_public_alias") {
		if _, err := s.db.ExecContext(ctx, `CREATE UNIQUE INDEX uq_accounts_public_alias ON accounts(public_alias)`); err != nil && !isDuplicateEntryErr(err) {
			return err
		}
	}
	s.accountAliasSchemaReady = true
	return nil
}

func (s *MySQLStore) ensureWebhookTypeSchema(ctx context.Context) error {
	s.webhookTypeSchemaMu.Lock()
	defer s.webhookTypeSchemaMu.Unlock()
	if s.webhookTypeSchemaReady {
		return nil
	}
	if s.indexExists("webhook_types", "uniq_account_type") {
		if _, err := s.db.ExecContext(ctx, `DROP INDEX uniq_account_type ON webhook_types`); err != nil {
			return err
		}
	}
	s.webhookTypeSchemaReady = true
	return nil
}

func (s *MySQLStore) ensureWebhookIdentitySchema(ctx context.Context) error {
	s.webhookIdentitySchemaMu.Lock()
	defer s.webhookIdentitySchemaMu.Unlock()
	if s.webhookIdentitySchemaReady {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS webhook_identities (
		id VARCHAR(64) PRIMARY KEY,
		account_id VARCHAR(64) NOT NULL,
		type_id VARCHAR(64) NOT NULL,
		secret_id VARCHAR(64) NOT NULL,
		public_alias VARCHAR(128) NOT NULL,
		secret_value VARCHAR(255) NOT NULL,
		local_part VARCHAR(384) NOT NULL,
		status VARCHAR(32) NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME NULL,
		UNIQUE KEY uq_webhook_identity_local_part (local_part),
		INDEX idx_webhook_identity_account_status (account_id, status),
		INDEX idx_webhook_identity_secret (secret_id)
	)`); err != nil {
		return err
	}
	s.webhookIdentitySchemaReady = true
	return nil
}

func (s *MySQLStore) CreateAccount(ctx context.Context, email string) (domain.Account, string, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, "", err
	}
	slug := slugFromEmail(email)
	acct, err := s.GetAccountBySlug(ctx, slug)
	if err == nil {
		t, terr := security.NewToken(24)
		if terr != nil {
			return domain.Account{}, "", terr
		}
		_, ierr := s.db.ExecContext(ctx, `INSERT INTO account_tokens(id, account_id, token_hash, created_at) VALUES(?,?,?,UTC_TIMESTAMP())`, uuid.NewString(), acct.ID, security.HashValue(t))
		if ierr != nil {
			return domain.Account{}, "", ierr
		}
		return acct, t, nil
	}
	id := uuid.NewString()
	token, terr := security.NewToken(24)
	if terr != nil {
		return domain.Account{}, "", terr
	}
	hash := security.HashValue(token)
	_, ierr := s.db.ExecContext(ctx, `INSERT INTO accounts(id, slug, public_alias, owner_email, created_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, id, slug, slug, email)
	if ierr != nil {
		return domain.Account{}, "", ierr
	}
	_, ierr = s.db.ExecContext(ctx, `INSERT INTO account_tokens(id, account_id, token_hash, created_at) VALUES(?,?,?,UTC_TIMESTAMP())`, uuid.NewString(), id, hash)
	if ierr != nil {
		return domain.Account{}, "", ierr
	}
	return domain.Account{ID: id, Slug: slug, PublicAlias: slug, OwnerEmail: email, TokenHash: hash, CreatedAt: time.Now().UTC()}, token, nil
}

func (s *MySQLStore) EnsureSingleTenantClaim(ctx context.Context, ownerEmail string, ttl time.Duration) (domain.SingleTenantClaim, string, bool, error) {
	now := time.Now().UTC()
	var existing domain.SingleTenantClaim
	err := s.db.QueryRowContext(ctx, `
SELECT id, owner_email, claim_hash, created_at, expires_at
FROM single_tenant_claims
WHERE owner_email=? AND consumed_at IS NULL AND expires_at > UTC_TIMESTAMP()
ORDER BY created_at DESC
LIMIT 1`, ownerEmail).Scan(&existing.ID, &existing.OwnerEmail, &existing.ClaimHash, &existing.CreatedAt, &existing.ExpiresAt)
	if err == nil {
		return existing, "", false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.SingleTenantClaim{}, "", false, err
	}

	code, err := security.NewToken(24)
	if err != nil {
		return domain.SingleTenantClaim{}, "", false, err
	}
	claim := domain.SingleTenantClaim{
		ID:         uuid.NewString(),
		OwnerEmail: ownerEmail,
		ClaimHash:  security.HashValue(code),
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO single_tenant_claims(id, owner_email, claim_hash, created_at, expires_at)
VALUES(?,?,?,?,?)`, claim.ID, claim.OwnerEmail, claim.ClaimHash, claim.CreatedAt, claim.ExpiresAt)
	if err != nil {
		return domain.SingleTenantClaim{}, "", false, err
	}
	return claim, code, true, nil
}

func (s *MySQLStore) ConsumeSingleTenantClaim(ctx context.Context, claimCode string) (domain.SingleTenantClaim, error) {
	claimHash := security.HashValue(strings.TrimSpace(claimCode))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SingleTenantClaim{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var claim domain.SingleTenantClaim
	var consumedAt sql.NullTime
	var consumedAccountID sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT id, owner_email, claim_hash, created_at, expires_at, consumed_at, consumed_account_id
FROM single_tenant_claims
WHERE claim_hash=?
LIMIT 1
FOR UPDATE`, claimHash).Scan(
		&claim.ID,
		&claim.OwnerEmail,
		&claim.ClaimHash,
		&claim.CreatedAt,
		&claim.ExpiresAt,
		&consumedAt,
		&consumedAccountID,
	)
	if err != nil {
		return domain.SingleTenantClaim{}, errors.New("claim not found")
	}
	if !hmac.Equal([]byte(claim.ClaimHash), []byte(claimHash)) {
		return domain.SingleTenantClaim{}, errors.New("claim not found")
	}
	if consumedAt.Valid || !claim.ExpiresAt.After(time.Now().UTC()) {
		return domain.SingleTenantClaim{}, errors.New("claim expired or already consumed")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE single_tenant_claims SET consumed_at=UTC_TIMESTAMP() WHERE id=?`, claim.ID); err != nil {
		return domain.SingleTenantClaim{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SingleTenantClaim{}, err
	}
	return claim, nil
}

func (s *MySQLStore) RecordSingleTenantClaimAccount(ctx context.Context, claimID, accountID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE single_tenant_claims SET consumed_account_id=? WHERE id=?`, accountID, claimID)
	return err
}

func (s *MySQLStore) GetAccountBySlug(ctx context.Context, slug string) (domain.Account, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, err
	}
	var a domain.Account
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, COALESCE(NULLIF(public_alias,''), slug), owner_email, created_at FROM accounts WHERE slug=? LIMIT 1`, slug).Scan(&a.ID, &a.Slug, &a.PublicAlias, &a.OwnerEmail, &a.CreatedAt)
	if err != nil {
		return domain.Account{}, err
	}
	return a, nil
}

func (s *MySQLStore) GetAccountByPublicAlias(ctx context.Context, alias string) (domain.Account, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, err
	}
	var a domain.Account
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, COALESCE(NULLIF(public_alias,''), slug), owner_email, created_at FROM accounts WHERE public_alias=? OR (COALESCE(public_alias,'')='' AND slug=?) LIMIT 1`, alias, alias).Scan(&a.ID, &a.Slug, &a.PublicAlias, &a.OwnerEmail, &a.CreatedAt)
	if err != nil {
		return domain.Account{}, err
	}
	return a, nil
}

func (s *MySQLStore) GetAccountByToken(ctx context.Context, token string) (domain.Account, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, err
	}
	h := security.HashValue(token)
	var a domain.Account
	err := s.db.QueryRowContext(ctx, `
SELECT a.id, a.slug, COALESCE(NULLIF(a.public_alias,''), a.slug), a.owner_email, a.created_at
FROM accounts a
JOIN account_tokens t ON t.account_id=a.id
WHERE t.token_hash=? AND t.revoked_at IS NULL
ORDER BY t.created_at DESC
LIMIT 1`, h).Scan(&a.ID, &a.Slug, &a.PublicAlias, &a.OwnerEmail, &a.CreatedAt)
	if err != nil {
		return domain.Account{}, errors.New("unauthorized")
	}
	return a, nil
}

func (s *MySQLStore) ListAccountTokens(ctx context.Context, accountID string) ([]domain.AccountToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, created_at FROM account_tokens WHERE account_id=? AND revoked_at IS NULL ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AccountToken
	for rows.Next() {
		var t domain.AccountToken
		if err := rows.Scan(&t.ID, &t.AccountID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *MySQLStore) RevokeAccountToken(ctx context.Context, accountID, tokenID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE account_tokens SET revoked_at=UTC_TIMESTAMP() WHERE id=? AND account_id=? AND revoked_at IS NULL`, tokenID, accountID)
	return err
}

func (s *MySQLStore) CreateWebhookType(ctx context.Context, accountID, typeKey, plainTextAction string, useLLMFallback bool) (domain.WebhookType, error) {
	if err := s.ensureWebhookTypeSchema(ctx); err != nil {
		return domain.WebhookType{}, err
	}
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_types(id, account_id, type_key, plain_text_action, use_llm_fallback, created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP())`, id, accountID, typeKey, plainTextAction, useLLMFallback)
	if err != nil {
		return domain.WebhookType{}, err
	}
	return domain.WebhookType{ID: id, AccountID: accountID, TypeKey: typeKey, PlainTextAction: plainTextAction, UseLLMFallback: useLLMFallback, CreatedAt: time.Now().UTC()}, nil
}

func (s *MySQLStore) GetAccount(ctx context.Context, id string) (domain.Account, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, err
	}
	var a domain.Account
	err := s.db.QueryRowContext(ctx, "SELECT id, slug, COALESCE(NULLIF(public_alias,''), slug), owner_email, created_at FROM accounts WHERE id=?", id).
		Scan(&a.ID, &a.Slug, &a.PublicAlias, &a.OwnerEmail, &a.CreatedAt)
	return a, err
}

func (s *MySQLStore) UpdateAccountPublicAlias(ctx context.Context, accountID, publicAlias string) (domain.Account, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.Account{}, err
	}
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.Account{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Account{}, err
	}
	defer tx.Rollback()
	current, err := s.getAccountTx(ctx, tx, accountID)
	if err != nil {
		return domain.Account{}, err
	}
	if err := s.ensureAliasChangeIdentityReservationTx(ctx, tx, accountID, current.PublicAlias, publicAlias); err != nil {
		return domain.Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET public_alias=? WHERE id=?`, publicAlias, accountID); err != nil {
		if isDuplicateEntryErr(err) {
			return domain.Account{}, errors.New("public alias already in use")
		}
		return domain.Account{}, err
	}
	if err := s.rekeyActiveWebhookIdentitiesTx(ctx, tx, accountID, current.PublicAlias, publicAlias); err != nil {
		return domain.Account{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Account{}, err
	}
	return s.GetAccount(ctx, accountID)
}

func (s *MySQLStore) getAccountTx(ctx context.Context, tx *sql.Tx, accountID string) (domain.Account, error) {
	var acct domain.Account
	err := tx.QueryRowContext(ctx, `SELECT id, slug, COALESCE(NULLIF(public_alias,''), slug), owner_email, created_at FROM accounts WHERE id=? LIMIT 1`, accountID).
		Scan(&acct.ID, &acct.Slug, &acct.PublicAlias, &acct.OwnerEmail, &acct.CreatedAt)
	return acct, err
}

func (s *MySQLStore) upsertWebhookIdentityTx(ctx context.Context, tx *sql.Tx, identity domain.WebhookIdentity) error {
	now := time.Now().UTC()
	if identity.ID == "" {
		identity.ID = uuid.NewString()
	}
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM webhook_identities WHERE local_part=? LIMIT 1`, identity.LocalPart).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && existingID != "" {
		_, err = tx.ExecContext(ctx, `UPDATE webhook_identities SET account_id=?, type_id=?, secret_id=?, public_alias=?, secret_value=?, status=?, updated_at=?, deleted_at=? WHERE id=?`,
			identity.AccountID, identity.TypeID, identity.SecretID, identity.PublicAlias, identity.SecretValue, identity.Status, now, identity.DeletedAt, existingID)
		return err
	}
	createdAt := identity.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO webhook_identities(id, account_id, type_id, secret_id, public_alias, secret_value, local_part, status, created_at, updated_at, deleted_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		identity.ID, identity.AccountID, identity.TypeID, identity.SecretID, identity.PublicAlias, identity.SecretValue, identity.LocalPart, identity.Status, createdAt, now, identity.DeletedAt)
	return err
}

func (s *MySQLStore) ensureAliasChangeIdentityReservationTx(ctx context.Context, tx *sql.Tx, accountID, oldAlias, newAlias string) error {
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(secret_value,'') FROM webhook_secrets WHERE account_id=? AND status='active'`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var secretValue string
		if err := rows.Scan(&secretValue); err != nil {
			return err
		}
		localPart := webhookid.BuildLocalPart(newAlias, secretValue)
		var existingAccountID string
		err := tx.QueryRowContext(ctx, `SELECT account_id FROM webhook_identities WHERE local_part=? LIMIT 1`, localPart).Scan(&existingAccountID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if existingAccountID != accountID {
			return domain.ErrWebhookIdentityReserved
		}
	}
	return nil
}

func (s *MySQLStore) rekeyActiveWebhookIdentitiesTx(ctx context.Context, tx *sql.Tx, accountID, oldAlias, newAlias string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, type_id, secret_id, COALESCE(secret_value,'') FROM webhook_identities WHERE account_id=? AND public_alias=? AND status='active'`, accountID, oldAlias)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id, typeID, secretID, secretValue string
	}
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.id, &item.typeID, &item.secretID, &item.secretValue); err != nil {
			return err
		}
		items = append(items, item)
	}
	now := time.Now().UTC()
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE webhook_identities SET status='deleted_tombstoned', updated_at=?, deleted_at=? WHERE id=?`, now, now, item.id); err != nil {
			return err
		}
		if err := s.upsertWebhookIdentityTx(ctx, tx, domain.WebhookIdentity{
			AccountID:   accountID,
			TypeID:      item.typeID,
			SecretID:    item.secretID,
			PublicAlias: newAlias,
			SecretValue: item.secretValue,
			LocalPart:   webhookid.BuildLocalPart(newAlias, item.secretValue),
			Status:      "active",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *MySQLStore) ListWebhookTypes(ctx context.Context, accountID string) ([]domain.WebhookType, error) {
	if err := s.ensureWebhookTypeSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, type_key, plain_text_action, use_llm_fallback, created_at FROM webhook_types WHERE account_id=? ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookType
	for rows.Next() {
		var w domain.WebhookType
		if err := rows.Scan(&w.ID, &w.AccountID, &w.TypeKey, &w.PlainTextAction, &w.UseLLMFallback, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

func (s *MySQLStore) GetWebhookTypeByAccountAndKey(ctx context.Context, accountID, typeKey string) (domain.WebhookType, error) {
	if err := s.ensureWebhookTypeSchema(ctx); err != nil {
		return domain.WebhookType{}, err
	}
	var w domain.WebhookType
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, plain_text_action, use_llm_fallback, created_at FROM webhook_types WHERE account_id=? AND type_key=? ORDER BY created_at DESC, id DESC LIMIT 1`, accountID, typeKey).Scan(&w.ID, &w.AccountID, &w.TypeKey, &w.PlainTextAction, &w.UseLLMFallback, &w.CreatedAt)
	if err != nil {
		return domain.WebhookType{}, err
	}
	return w, nil
}

func (s *MySQLStore) GetWebhookTypeByID(ctx context.Context, typeID string) (domain.WebhookType, error) {
	var w domain.WebhookType
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, plain_text_action, use_llm_fallback, created_at FROM webhook_types WHERE id=? LIMIT 1`, typeID).Scan(&w.ID, &w.AccountID, &w.TypeKey, &w.PlainTextAction, &w.UseLLMFallback, &w.CreatedAt)
	if err != nil {
		return domain.WebhookType{}, err
	}
	return w, nil
}

func (s *MySQLStore) DeleteWebhookType(ctx context.Context, accountID, typeID string) error {
	if err := s.ensureWebhookTypeSchema(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM webhook_types WHERE id=? AND account_id=?`, typeID, accountID)
	return err
}

func (s *MySQLStore) CreateSecret(ctx context.Context, accountID, typeID string) (domain.WebhookSecret, string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		raw, err := security.NewToken(18)
		if err != nil {
			return domain.WebhookSecret{}, "", err
		}
		secret, err := s.CreateSecretWithValue(ctx, accountID, typeID, raw)
		if err == nil {
			return secret, raw, nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "already in use") {
			return domain.WebhookSecret{}, "", err
		}
	}
	return domain.WebhookSecret{}, "", errors.New("failed to generate unique secret")
}

func (s *MySQLStore) CreateSecretWithValue(ctx context.Context, accountID, typeID, secretValue string) (domain.WebhookSecret, error) {
	if err := s.ensureAccountAliasSchema(ctx); err != nil {
		return domain.WebhookSecret{}, err
	}
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.WebhookSecret{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.WebhookSecret{}, err
	}
	defer tx.Rollback()

	acct, err := s.getAccountTx(ctx, tx, accountID)
	if err != nil {
		return domain.WebhookSecret{}, err
	}
	localPart := webhookid.BuildLocalPart(acct.PublicAlias, secretValue)
	var identity domain.WebhookIdentity
	err = tx.QueryRowContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, COALESCE(secret_value,''), local_part, status, created_at, updated_at, deleted_at FROM webhook_identities WHERE local_part=? LIMIT 1`, localPart).
		Scan(&identity.ID, &identity.AccountID, &identity.TypeID, &identity.SecretID, &identity.PublicAlias, &identity.SecretValue, &identity.LocalPart, &identity.Status, &identity.CreatedAt, &identity.UpdatedAt, &identity.DeletedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.WebhookSecret{}, err
	}
	if err == nil {
		if identity.AccountID != accountID {
			return domain.WebhookSecret{}, domain.ErrWebhookIdentityReserved
		}
		if identity.Status == "active" {
			return domain.WebhookSecret{}, domain.ErrWebhookIdentityAlreadyActive
		}
	}

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO webhook_secrets(id, account_id, type_id, secret_value, status, created_at) VALUES(?,?,?,?, 'active', UTC_TIMESTAMP())`, id, accountID, typeID, secretValue); err != nil {
		return domain.WebhookSecret{}, err
	}
	if err := s.upsertWebhookIdentityTx(ctx, tx, domain.WebhookIdentity{
		ID:          identity.ID,
		AccountID:   accountID,
		TypeID:      typeID,
		SecretID:    id,
		PublicAlias: acct.PublicAlias,
		SecretValue: secretValue,
		LocalPart:   localPart,
		Status:      "active",
	}); err != nil {
		return domain.WebhookSecret{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.WebhookSecret{}, err
	}
	return domain.WebhookSecret{ID: id, AccountID: accountID, TypeID: typeID, SecretValue: secretValue, Status: "active", CreatedAt: time.Now().UTC()}, nil
}

func (s *MySQLStore) ListSecrets(ctx context.Context, accountID, typeID string) ([]domain.WebhookSecret, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, type_id, COALESCE(secret_value, ''), status, created_at FROM webhook_secrets WHERE account_id=? AND type_id=? AND status='active' ORDER BY created_at DESC`, accountID, typeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookSecret
	for rows.Next() {
		var sec domain.WebhookSecret
		if err := rows.Scan(&sec.ID, &sec.AccountID, &sec.TypeID, &sec.SecretValue, &sec.Status, &sec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, nil
}

func (s *MySQLStore) DeleteSecret(ctx context.Context, accountID, secretID string) error {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE webhook_secrets SET status='revoked' WHERE id=? AND account_id=?`, secretID, accountID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE webhook_identities SET status='deleted_tombstoned', updated_at=?, deleted_at=? WHERE secret_id=? AND account_id=? AND status IN ('active','blocked')`, now, now, secretID, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MySQLStore) ValidateSecret(ctx context.Context, accountID, typeID, secret string) (domain.WebhookSecret, error) {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.WebhookSecret{}, err
	}
	var sec domain.WebhookSecret
	err := s.db.QueryRowContext(ctx, `SELECT ws.id, ws.account_id, ws.type_id, COALESCE(ws.secret_value, ''), ws.status, ws.created_at
		FROM webhook_secrets ws
		INNER JOIN accounts a ON a.id = ws.account_id
		INNER JOIN webhook_identities wi
			ON wi.account_id = ws.account_id
			AND wi.secret_id = ws.id
			AND wi.local_part = CONCAT(COALESCE(NULLIF(a.public_alias,''), a.slug), '.', ws.secret_value)
			AND wi.status='active'
		WHERE ws.account_id=? AND ws.type_id=? AND ws.secret_value=? AND ws.status='active'
		LIMIT 1`, accountID, typeID, secret).Scan(&sec.ID, &sec.AccountID, &sec.TypeID, &sec.SecretValue, &sec.Status, &sec.CreatedAt)
	if err != nil {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
}

func (s *MySQLStore) ResolveSecretAnyType(ctx context.Context, accountID, secret string) (domain.WebhookSecret, error) {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.WebhookSecret{}, err
	}
	var sec domain.WebhookSecret
	err := s.db.QueryRowContext(ctx, `SELECT ws.id, ws.account_id, ws.type_id, COALESCE(ws.secret_value, ''), ws.status, ws.created_at
		FROM webhook_secrets ws
		INNER JOIN accounts a ON a.id = ws.account_id
		INNER JOIN webhook_identities wi
			ON wi.account_id = ws.account_id
			AND wi.secret_id = ws.id
			AND wi.local_part = CONCAT(COALESCE(NULLIF(a.public_alias,''), a.slug), '.', ws.secret_value)
			AND wi.status='active'
		WHERE ws.account_id=? AND ws.secret_value=? AND ws.status='active'
		LIMIT 1`, accountID, secret).Scan(&sec.ID, &sec.AccountID, &sec.TypeID, &sec.SecretValue, &sec.Status, &sec.CreatedAt)
	if err != nil {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
}

func (s *MySQLStore) ListWebhookIdentities(ctx context.Context, accountID string) ([]domain.WebhookIdentity, error) {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, COALESCE(secret_value,''), local_part, status, created_at, updated_at, deleted_at FROM webhook_identities WHERE account_id=? ORDER BY updated_at DESC, created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookIdentity
	for rows.Next() {
		var item domain.WebhookIdentity
		if err := rows.Scan(&item.ID, &item.AccountID, &item.TypeID, &item.SecretID, &item.PublicAlias, &item.SecretValue, &item.LocalPart, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *MySQLStore) GetWebhookIdentityByLocalPart(ctx context.Context, localPart string) (domain.WebhookIdentity, error) {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.WebhookIdentity{}, err
	}
	var item domain.WebhookIdentity
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, COALESCE(secret_value,''), local_part, status, created_at, updated_at, deleted_at FROM webhook_identities WHERE local_part=? LIMIT 1`, webhookid.NormalizeWebhookSecret(localPart)).
		Scan(&item.ID, &item.AccountID, &item.TypeID, &item.SecretID, &item.PublicAlias, &item.SecretValue, &item.LocalPart, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.WebhookIdentity{}, domain.ErrWebhookIdentityNotFound
		}
		return domain.WebhookIdentity{}, err
	}
	return item, nil
}

func (s *MySQLStore) UpdateWebhookIdentityStatus(ctx context.Context, accountID, identityID, status string) (domain.WebhookIdentity, error) {
	if err := s.ensureWebhookIdentitySchema(ctx); err != nil {
		return domain.WebhookIdentity{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.WebhookIdentity{}, err
	}
	defer tx.Rollback()
	var item domain.WebhookIdentity
	err = tx.QueryRowContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, COALESCE(secret_value,''), local_part, status, created_at, updated_at, deleted_at FROM webhook_identities WHERE id=? AND account_id=? LIMIT 1`, identityID, accountID).
		Scan(&item.ID, &item.AccountID, &item.TypeID, &item.SecretID, &item.PublicAlias, &item.SecretValue, &item.LocalPart, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.WebhookIdentity{}, domain.ErrWebhookIdentityNotFound
		}
		return domain.WebhookIdentity{}, err
	}
	now := time.Now().UTC()
	switch status {
	case "blocked":
		if _, err := tx.ExecContext(ctx, `UPDATE webhook_identities SET status='blocked', updated_at=?, deleted_at=NULL WHERE id=? AND account_id=?`, now, identityID, accountID); err != nil {
			return domain.WebhookIdentity{}, err
		}
	case "active":
		if item.Status == "deleted_tombstoned" {
			var activeSecretID string
			if err := tx.QueryRowContext(ctx, `SELECT id FROM webhook_secrets WHERE account_id=? AND secret_value=? AND status='active' ORDER BY created_at DESC LIMIT 1`, accountID, item.SecretValue).Scan(&activeSecretID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return domain.WebhookIdentity{}, errors.New("matching active secret required to restore deleted identity")
				}
				return domain.WebhookIdentity{}, err
			}
			item.SecretID = activeSecretID
		}
		if _, err := tx.ExecContext(ctx, `UPDATE webhook_identities SET status='active', secret_id=?, updated_at=?, deleted_at=NULL WHERE id=? AND account_id=?`, item.SecretID, now, identityID, accountID); err != nil {
			return domain.WebhookIdentity{}, err
		}
	default:
		return domain.WebhookIdentity{}, errors.New("unsupported identity status")
	}
	if err := tx.Commit(); err != nil {
		return domain.WebhookIdentity{}, err
	}
	return s.GetWebhookIdentityByLocalPart(ctx, item.LocalPart)
}

func (s *MySQLStore) CreateForwardTarget(ctx context.Context, accountID, targetType, configJSON string) (domain.ForwardTarget, error) {
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `INSERT INTO forward_targets(id, account_id, target_type, config_json, created_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, id, accountID, targetType, configJSON)
	if err != nil {
		return domain.ForwardTarget{}, err
	}
	return domain.ForwardTarget{ID: id, AccountID: accountID, TargetType: targetType, ConfigJSON: configJSON, CreatedAt: time.Now().UTC()}, nil
}

func (s *MySQLStore) ListForwardTargets(ctx context.Context, accountID string) ([]domain.ForwardTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, target_type, config_json, created_at FROM forward_targets WHERE account_id=? ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ForwardTarget
	for rows.Next() {
		var t domain.ForwardTarget
		if err := rows.Scan(&t.ID, &t.AccountID, &t.TargetType, &t.ConfigJSON, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *MySQLStore) UpdateForwardTarget(ctx context.Context, target domain.ForwardTarget) (domain.ForwardTarget, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE forward_targets SET target_type=?, config_json=? WHERE id=? AND account_id=?`, target.TargetType, target.ConfigJSON, target.ID, target.AccountID)
	if err != nil {
		return domain.ForwardTarget{}, err
	}
	var updated domain.ForwardTarget
	err = s.db.QueryRowContext(ctx, `SELECT id, account_id, target_type, config_json, created_at FROM forward_targets WHERE id=? AND account_id=? LIMIT 1`, target.ID, target.AccountID).
		Scan(&updated.ID, &updated.AccountID, &updated.TargetType, &updated.ConfigJSON, &updated.CreatedAt)
	if err != nil {
		return domain.ForwardTarget{}, err
	}
	return updated, nil
}

func (s *MySQLStore) DeleteForwardTarget(ctx context.Context, accountID, targetID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM forward_targets WHERE id=? AND account_id=?`, targetID, accountID)
	return err
}

func integrationSecretSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS integration_secrets (
			id VARCHAR(36) PRIMARY KEY,
			account_id VARCHAR(36) NOT NULL,
			secret_key VARCHAR(128) NOT NULL,
			purpose TEXT NULL,
			secret_value TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uq_integration_secret_account_key (account_id, secret_key)
		)`,
	}
}

func (s *MySQLStore) ensureIntegrationSecretSchema(ctx context.Context) error {
	s.integrationSecretSchemaMu.Lock()
	defer s.integrationSecretSchemaMu.Unlock()
	if s.integrationSecretSchemaReady {
		return nil
	}
	for _, stmt := range integrationSecretSchemaStatements() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	s.integrationSecretSchemaReady = true
	return nil
}

func (s *MySQLStore) CreateIntegrationSecret(ctx context.Context, secret domain.IntegrationSecret) (domain.IntegrationSecret, error) {
	if err := s.ensureIntegrationSecretSchema(ctx); err != nil {
		return domain.IntegrationSecret{}, err
	}
	if secret.ID == "" {
		secret.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO integration_secrets(id, account_id, secret_key, purpose, secret_value, created_at, updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`,
		secret.ID, secret.AccountID, secret.SecretKey, secret.Purpose, secret.SecretValue)
	if err != nil {
		return domain.IntegrationSecret{}, err
	}
	secret.CreatedAt = time.Now().UTC()
	secret.UpdatedAt = secret.CreatedAt
	secret.SecretValue = ""
	return secret, nil
}

func (s *MySQLStore) ListIntegrationSecrets(ctx context.Context, accountID string) ([]domain.IntegrationSecret, error) {
	if err := s.ensureIntegrationSecretSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, secret_key, COALESCE(purpose,''), created_at, updated_at FROM integration_secrets WHERE account_id=? ORDER BY created_at ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IntegrationSecret
	for rows.Next() {
		var secret domain.IntegrationSecret
		if err := rows.Scan(&secret.ID, &secret.AccountID, &secret.SecretKey, &secret.Purpose, &secret.CreatedAt, &secret.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, secret)
	}
	return out, nil
}

func (s *MySQLStore) UpdateIntegrationSecret(ctx context.Context, secret domain.IntegrationSecret) (domain.IntegrationSecret, error) {
	if err := s.ensureIntegrationSecretSchema(ctx); err != nil {
		return domain.IntegrationSecret{}, err
	}
	if strings.TrimSpace(secret.SecretValue) == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE integration_secrets SET secret_key=?, purpose=?, updated_at=UTC_TIMESTAMP() WHERE id=? AND account_id=?`,
			secret.SecretKey, secret.Purpose, secret.ID, secret.AccountID)
		if err != nil {
			return domain.IntegrationSecret{}, err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `UPDATE integration_secrets SET secret_key=?, purpose=?, secret_value=?, updated_at=UTC_TIMESTAMP() WHERE id=? AND account_id=?`,
			secret.SecretKey, secret.Purpose, secret.SecretValue, secret.ID, secret.AccountID)
		if err != nil {
			return domain.IntegrationSecret{}, err
		}
	}
	var updated domain.IntegrationSecret
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, secret_key, COALESCE(purpose,''), created_at, updated_at FROM integration_secrets WHERE id=? AND account_id=? LIMIT 1`, secret.ID, secret.AccountID).
		Scan(&updated.ID, &updated.AccountID, &updated.SecretKey, &updated.Purpose, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return domain.IntegrationSecret{}, err
	}
	return updated, nil
}

func (s *MySQLStore) DeleteIntegrationSecret(ctx context.Context, accountID, secretID string) error {
	if err := s.ensureIntegrationSecretSchema(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM integration_secrets WHERE id=? AND account_id=?`, secretID, accountID)
	return err
}

func (s *MySQLStore) ResolveIntegrationSecretValue(ctx context.Context, accountID, secretKey string) (string, error) {
	if err := s.ensureIntegrationSecretSchema(ctx); err != nil {
		return "", err
	}
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT secret_value FROM integration_secrets WHERE account_id=? AND secret_key=? LIMIT 1`, accountID, secretKey).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *MySQLStore) CreateEvent(ctx context.Context, e domain.WebhookEvent) (domain.WebhookEvent, error) {
	e.ID = uuid.NewString()
	e.CreatedAt = time.Now().UTC()
	modernQuery := `INSERT INTO webhook_events(id, account_id, type_id, secret_id, request_id, source_event_id, type_key, raw_payload_json, payload_json, processed_text, action_selected, status, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?, ?,UTC_TIMESTAMP())`
	modernArgs := []interface{}{e.ID, e.AccountID, e.TypeID, e.SecretID, e.RequestID, nullIfEmpty(e.SourceEventID), e.TypeKey, nullIfEmpty(e.RawPayloadJSON), e.PayloadJSON, nullIfEmpty(e.ProcessedText), e.ActionSelected, e.Status}
	_, err := s.db.ExecContext(ctx, modernQuery, modernArgs...)
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			_, err = s.db.ExecContext(ctx, modernQuery, modernArgs...)
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed account_id=%s event_id=%s err=%v", e.AccountID, e.ID, schemaErr)
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		legacyQuery := `INSERT INTO webhook_events(id, account_id, type_id, secret_id, request_id, source_event_id, type_key, payload_json, action_selected, status, created_at) VALUES(?,?,?,?,?,?,?,?,?, ?,UTC_TIMESTAMP())`
		legacyArgs := []interface{}{e.ID, e.AccountID, e.TypeID, e.SecretID, e.RequestID, nullIfEmpty(e.SourceEventID), e.TypeKey, e.PayloadJSON, e.ActionSelected, e.Status}
		_, err = s.db.ExecContext(ctx, legacyQuery, legacyArgs...)
	}
	if err != nil {
		return domain.WebhookEvent{}, err
	}
	return e, nil
}

func (s *MySQLStore) UpdateEventStatus(ctx context.Context, eventID, status, action string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_events SET status=?, action_selected=? WHERE id=?`, status, action, eventID)
	return err
}

func (s *MySQLStore) UpdateEventProcessedText(ctx context.Context, eventID, processedText string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_events SET processed_text=? WHERE id=?`, nullIfEmpty(processedText), eventID)
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			_, err = s.db.ExecContext(ctx, `UPDATE webhook_events SET processed_text=? WHERE id=?`, nullIfEmpty(processedText), eventID)
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed event_id=%s err=%v", eventID, schemaErr)
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		log.Printf("mysql.update_event_processed_text_skipped event_id=%s reason=processed_text_column_unavailable", eventID)
		return nil
	}
	return err
}

func buildEventSelectQuery(whereClause string, includeRaw, includeProcessed, includeTags bool) string {
	rawExpr := `''`
	if includeRaw {
		rawExpr = `COALESCE(raw_payload_json,'')`
	}
	processedExpr := `''`
	if includeProcessed {
		processedExpr = `COALESCE(processed_text,'')`
	}
	tagsExpr := `''`
	if includeTags {
		tagsExpr = `COALESCE(tags_json,'')`
	}
	return `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, ` +
		rawExpr + `, payload_json, ` + processedExpr + `, action_selected, ` + tagsExpr + `, status, created_at FROM webhook_events ` + whereClause
}

func eventSelectVariants() []eventSelectVariant {
	return []eventSelectVariant{
		{includeRaw: true, includeProcessed: true, includeTags: true},
		{includeRaw: true, includeProcessed: true, includeTags: false},
		{includeRaw: false, includeProcessed: true, includeTags: true},
		{includeRaw: false, includeProcessed: true, includeTags: false},
		{includeRaw: true, includeProcessed: false, includeTags: false},
		{includeRaw: false, includeProcessed: false, includeTags: false},
	}
}

func webhookEventSchemaStatements() []string {
	return []string{
		`ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS source_event_id VARCHAR(128) NULL AFTER request_id`,
		`ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS raw_payload_json JSON NULL AFTER type_key`,
		`ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS processed_text TEXT NULL AFTER payload_json`,
		`ALTER TABLE webhook_events ADD COLUMN IF NOT EXISTS tags_json TEXT NULL AFTER action_selected`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_events_account_source ON webhook_events (account_id, source_event_id)`,
	}
}

func (s *MySQLStore) ensureWebhookEventSchema(ctx context.Context) error {
	s.eventSchemaMu.Lock()
	defer s.eventSchemaMu.Unlock()
	if s.eventSchemaReady {
		return nil
	}
	for _, stmt := range webhookEventSchemaStatements() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	s.eventSchemaReady = true
	return nil
}

func (s *MySQLStore) ListEvents(ctx context.Context, accountID string, limit int) ([]domain.WebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	for _, variant := range eventSelectVariants() {
		query := buildEventSelectQuery(`WHERE account_id=? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
		rows, err = s.db.QueryContext(ctx, query, accountID, limit)
		if err == nil || !isUnknownColumnErr(err) {
			break
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			for _, variant := range eventSelectVariants() {
				query := buildEventSelectQuery(`WHERE account_id=? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
				rows, err = s.db.QueryContext(ctx, query, accountID, limit)
				if err == nil || !isUnknownColumnErr(err) {
					break
				}
			}
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed account_id=%s err=%v", accountID, schemaErr)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookEvent
	for rows.Next() {
		var e domain.WebhookEvent
		if err := rows.Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *MySQLStore) GetEvent(ctx context.Context, accountID, eventID string) (domain.WebhookEvent, error) {
	var e domain.WebhookEvent
	var err error
	for _, variant := range eventSelectVariants() {
		query := buildEventSelectQuery(`WHERE account_id=? AND id=?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
		err = s.db.QueryRowContext(ctx, query, accountID, eventID).
			Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt)
		if err == nil || !isUnknownColumnErr(err) {
			break
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			for _, variant := range eventSelectVariants() {
				query := buildEventSelectQuery(`WHERE account_id=? AND id=?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
				err = s.db.QueryRowContext(ctx, query, accountID, eventID).
					Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt)
				if err == nil || !isUnknownColumnErr(err) {
					break
				}
			}
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed account_id=%s event_id=%s err=%v", accountID, eventID, schemaErr)
		}
	}
	return e, err
}

func (s *MySQLStore) FindEventBySourceEventID(ctx context.Context, accountID, sourceEventID string) (domain.WebhookEvent, error) {
	if strings.TrimSpace(sourceEventID) == "" {
		return domain.WebhookEvent{}, errors.New("source event id required")
	}
	var e domain.WebhookEvent
	var err error
	for _, variant := range eventSelectVariants() {
		query := buildEventSelectQuery(`WHERE account_id=? AND source_event_id=? LIMIT 1`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
		err = s.db.QueryRowContext(ctx, query, accountID, sourceEventID).
			Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt)
		if err == nil || !isUnknownColumnErr(err) {
			break
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			for _, variant := range eventSelectVariants() {
				query := buildEventSelectQuery(`WHERE account_id=? AND source_event_id=? LIMIT 1`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
				err = s.db.QueryRowContext(ctx, query, accountID, sourceEventID).
					Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt)
				if err == nil || !isUnknownColumnErr(err) {
					break
				}
			}
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed account_id=%s source_event_id=%s err=%v", accountID, sourceEventID, schemaErr)
		}
	}
	if err != nil {
		return domain.WebhookEvent{}, err
	}
	return e, nil
}

func (s *MySQLStore) ListEventsByTag(ctx context.Context, accountID, tag string, limit int) ([]domain.WebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	for _, variant := range eventSelectVariants() {
		if !variant.includeTags {
			continue
		}
		query := buildEventSelectQuery(`WHERE account_id=? AND tags_json LIKE ? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
		rows, err = s.db.QueryContext(ctx, query, accountID, `%"`+tag+`"%`, limit)
		if err == nil || !isUnknownColumnErr(err) {
			break
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			for _, variant := range eventSelectVariants() {
				if !variant.includeTags {
					continue
				}
				query := buildEventSelectQuery(`WHERE account_id=? AND tags_json LIKE ? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
				rows, err = s.db.QueryContext(ctx, query, accountID, `%"`+tag+`"%`, limit)
				if err == nil || !isUnknownColumnErr(err) {
					break
				}
			}
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed account_id=%s tag=%s err=%v", accountID, tag, schemaErr)
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		log.Printf("mysql.list_events_by_tag_skipped account_id=%s tag=%s reason=tags_json_column_unavailable", accountID, tag)
		return []domain.WebhookEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookEvent
	for rows.Next() {
		var e domain.WebhookEvent
		if err := rows.Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *MySQLStore) ListEventsByTime(ctx context.Context, accountID string, since time.Time, limit int) ([]domain.WebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	for _, variant := range eventSelectVariants() {
		query := buildEventSelectQuery(`WHERE account_id=? AND created_at >= ? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
		rows, err = s.db.QueryContext(ctx, query, accountID, since, limit)
		if err == nil || !isUnknownColumnErr(err) {
			break
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			for _, variant := range eventSelectVariants() {
				query := buildEventSelectQuery(`WHERE account_id=? AND created_at >= ? ORDER BY created_at DESC LIMIT ?`, variant.includeRaw, variant.includeProcessed, variant.includeTags)
				rows, err = s.db.QueryContext(ctx, query, accountID, since, limit)
				if err == nil || !isUnknownColumnErr(err) {
					break
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WebhookEvent
	for rows.Next() {
		var e domain.WebhookEvent
		if err := rows.Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *MySQLStore) UpdateEventTags(ctx context.Context, eventID, tagsJSON string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_events SET tags_json=? WHERE id=?`, tagsJSON, eventID)
	if err != nil && isUnknownColumnErr(err) {
		if schemaErr := s.ensureWebhookEventSchema(ctx); schemaErr == nil {
			_, err = s.db.ExecContext(ctx, `UPDATE webhook_events SET tags_json=? WHERE id=?`, tagsJSON, eventID)
		} else {
			log.Printf("mysql.ensure_webhook_event_schema_failed event_id=%s err=%v", eventID, schemaErr)
		}
	}
	if err != nil && isUnknownColumnErr(err) {
		return nil
	}
	return err
}

func isUnknownColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown column")
}

func isDuplicateEntryErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") || strings.Contains(msg, "duplicate key")
}

func (s *MySQLStore) CreateTypeSignature(ctx context.Context, sig domain.WebhookTypeSignature) (domain.WebhookTypeSignature, error) {
	if sig.ID == "" {
		sig.ID = uuid.NewString()
	}
	if sig.Version <= 0 {
		sig.Version = 1
	}
	if sig.ConfidenceThreshold <= 0 {
		sig.ConfidenceThreshold = 0.75
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_type_signatures(id, account_id, type_key, version, required_keys_json, shape_hints_json, header_hints_json, confidence_threshold, enabled, source, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP())`,
		sig.ID, sig.AccountID, sig.TypeKey, sig.Version, sig.RequiredKeysJSON, sig.ShapeHintsJSON, sig.HeaderHintsJSON, sig.ConfidenceThreshold, sig.Enabled, sig.Source)
	if err != nil {
		return domain.WebhookTypeSignature{}, err
	}
	sig.CreatedAt = time.Now().UTC()
	return sig, nil
}

func (s *MySQLStore) ListTypeSignatures(ctx context.Context, accountID string) ([]domain.WebhookTypeSignature, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, type_key, version, required_keys_json, shape_hints_json, header_hints_json, confidence_threshold, enabled, source, created_at FROM webhook_type_signatures WHERE account_id=? AND enabled=1 ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WebhookTypeSignature{}
	for rows.Next() {
		var sgn domain.WebhookTypeSignature
		if err := rows.Scan(&sgn.ID, &sgn.AccountID, &sgn.TypeKey, &sgn.Version, &sgn.RequiredKeysJSON, &sgn.ShapeHintsJSON, &sgn.HeaderHintsJSON, &sgn.ConfidenceThreshold, &sgn.Enabled, &sgn.Source, &sgn.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sgn)
	}
	return out, nil
}

func (s *MySQLStore) GetLatestCandidateSignature(ctx context.Context, accountID, typeKey string) (domain.WebhookTypeSignature, error) {
	var sig domain.WebhookTypeSignature
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, version, required_keys_json, shape_hints_json, header_hints_json, confidence_threshold, enabled, source, created_at
FROM webhook_type_signatures
WHERE account_id=? AND type_key=? AND (source LIKE 'llm_candidate%' OR source LIKE 'autopromote%')
ORDER BY created_at DESC
LIMIT 1`, accountID, typeKey).
		Scan(&sig.ID, &sig.AccountID, &sig.TypeKey, &sig.Version, &sig.RequiredKeysJSON, &sig.ShapeHintsJSON, &sig.HeaderHintsJSON, &sig.ConfidenceThreshold, &sig.Enabled, &sig.Source, &sig.CreatedAt)
	if err != nil {
		return domain.WebhookTypeSignature{}, err
	}
	return sig, nil
}

func (s *MySQLStore) SetTypeSignatureEnabled(ctx context.Context, signatureID string, enabled bool, source string) error {
	if strings.TrimSpace(source) == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE webhook_type_signatures SET enabled=? WHERE id=?`, enabled, signatureID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_type_signatures SET enabled=?, source=? WHERE id=?`, enabled, source, signatureID)
	return err
}

func (s *MySQLStore) CreateTransform(ctx context.Context, tr domain.WebhookTransform) (domain.WebhookTransform, error) {
	if tr.ID == "" {
		tr.ID = uuid.NewString()
	}
	if tr.Version <= 0 {
		tr.Version = 1
	}
	if tr.Status == "" {
		tr.Status = "pending"
	}
	if strings.TrimSpace(tr.DeterministicTestsJSON) == "" {
		tr.DeterministicTestsJSON = `[]`
	}
	if strings.TrimSpace(tr.DSLText) == "" {
		tr.DSLText = `{}`
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_transforms(id, account_id, type_key, version, engine, wasm_blob_ref, dsl_text, deterministic_tests_json, status, created_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP())`,
		tr.ID, tr.AccountID, tr.TypeKey, tr.Version, tr.Engine, tr.WASMBlobRef, tr.DSLText, tr.DeterministicTestsJSON, tr.Status)
	if err != nil {
		return domain.WebhookTransform{}, err
	}
	tr.CreatedAt = time.Now().UTC()
	return tr, nil
}

func (s *MySQLStore) ListTransforms(ctx context.Context, accountID, typeKey string) ([]domain.WebhookTransform, error) {
	query := `SELECT id, account_id, type_key, version, engine, wasm_blob_ref, dsl_text, deterministic_tests_json, status, created_at FROM webhook_transforms WHERE account_id=?`
	args := []interface{}{accountID}
	if typeKey != "" {
		query += ` AND type_key=?`
		args = append(args, typeKey)
	}
	query += ` ORDER BY version DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WebhookTransform{}
	for rows.Next() {
		var tr domain.WebhookTransform
		if err := rows.Scan(&tr.ID, &tr.AccountID, &tr.TypeKey, &tr.Version, &tr.Engine, &tr.WASMBlobRef, &tr.DSLText, &tr.DeterministicTestsJSON, &tr.Status, &tr.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, nil
}

func (s *MySQLStore) GetActiveTransform(ctx context.Context, accountID, typeKey string) (domain.WebhookTransform, error) {
	var tr domain.WebhookTransform
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, version, engine, wasm_blob_ref, dsl_text, deterministic_tests_json, status, created_at FROM webhook_transforms WHERE account_id=? AND type_key=? AND status='active' ORDER BY version DESC LIMIT 1`, accountID, typeKey).
		Scan(&tr.ID, &tr.AccountID, &tr.TypeKey, &tr.Version, &tr.Engine, &tr.WASMBlobRef, &tr.DSLText, &tr.DeterministicTestsJSON, &tr.Status, &tr.CreatedAt)
	if err != nil {
		return domain.WebhookTransform{}, err
	}
	return tr, nil
}

func (s *MySQLStore) GetLatestTransformByStatus(ctx context.Context, accountID, typeKey, status string) (domain.WebhookTransform, error) {
	var tr domain.WebhookTransform
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, version, engine, wasm_blob_ref, dsl_text, deterministic_tests_json, status, created_at
FROM webhook_transforms
WHERE account_id=? AND type_key=? AND status=?
ORDER BY version DESC, created_at DESC
LIMIT 1`, accountID, typeKey, status).
		Scan(&tr.ID, &tr.AccountID, &tr.TypeKey, &tr.Version, &tr.Engine, &tr.WASMBlobRef, &tr.DSLText, &tr.DeterministicTestsJSON, &tr.Status, &tr.CreatedAt)
	if err != nil {
		return domain.WebhookTransform{}, err
	}
	return tr, nil
}

func (s *MySQLStore) SetTransformStatus(ctx context.Context, transformID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_transforms SET status=? WHERE id=?`, status, transformID)
	return err
}

func (s *MySQLStore) LogTransformRun(ctx context.Context, run domain.TransformRun) (domain.TransformRun, error) {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_transform_runs(id, event_id, account_id, type_key, transform_version, duration_ms, result_hash, error_text, created_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP())`,
		run.ID, run.EventID, run.AccountID, run.TypeKey, run.TransformVersion, run.DurationMS, run.ResultHash, run.ErrorText)
	if err != nil {
		return domain.TransformRun{}, err
	}
	run.CreatedAt = time.Now().UTC()
	return run, nil
}

func (s *MySQLStore) UpsertMasterPromptPolicy(ctx context.Context, accountID, promptText, updatedBy string) (domain.MasterPromptPolicy, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO master_prompt_policies(account_id, prompt_text, updated_by, updated_at, created_at)
VALUES(?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE prompt_text=VALUES(prompt_text), updated_by=VALUES(updated_by), updated_at=UTC_TIMESTAMP()`,
		accountID, promptText, updatedBy)
	if err != nil {
		return domain.MasterPromptPolicy{}, err
	}
	return s.GetMasterPromptPolicy(ctx, accountID)
}

func (s *MySQLStore) GetMasterPromptPolicy(ctx context.Context, accountID string) (domain.MasterPromptPolicy, error) {
	var p domain.MasterPromptPolicy
	err := s.db.QueryRowContext(ctx, `SELECT account_id, prompt_text, updated_by, updated_at, created_at FROM master_prompt_policies WHERE account_id=? LIMIT 1`, accountID).
		Scan(&p.AccountID, &p.PromptText, &p.UpdatedBy, &p.UpdatedAt, &p.CreatedAt)
	if err != nil {
		return domain.MasterPromptPolicy{}, err
	}
	return p, nil
}

func (s *MySQLStore) CreateWebhookSkill(ctx context.Context, skill domain.WebhookSkill) (domain.WebhookSkill, error) {
	if skill.ID == "" {
		skill.ID = uuid.NewString()
	}
	if skill.Priority == 0 {
		skill.Priority = 100
	}
	if strings.TrimSpace(skill.MemoryWriteMode) == "" {
		skill.MemoryWriteMode = "update_or_insert"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO webhook_skills(id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at)
VALUES(?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP())`,
		skill.ID, skill.AccountID, skill.TypeKey, skill.SkillKey, skill.SkillPrompt, skill.MatchContains, skill.ForcedAction, skill.MemoryWriteMode, skill.Priority, skill.Enabled)
	if err != nil {
		return domain.WebhookSkill{}, err
	}
	skill.CreatedAt = time.Now().UTC()
	return skill, nil
}

func (s *MySQLStore) ListWebhookSkills(ctx context.Context, accountID, typeKey string) ([]domain.WebhookSkill, error) {
	return s.listWebhookSkills(ctx, accountID, typeKey, false)
}

func (s *MySQLStore) ListWebhookSkillsIncludingDisabled(ctx context.Context, accountID, typeKey string) ([]domain.WebhookSkill, error) {
	return s.listWebhookSkills(ctx, accountID, typeKey, true)
}

func (s *MySQLStore) listWebhookSkills(ctx context.Context, accountID, typeKey string, includeDisabled bool) ([]domain.WebhookSkill, error) {
	query := `
SELECT id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at
FROM webhook_skills
WHERE account_id=? AND type_key=?`
	if !includeDisabled {
		query += ` AND enabled=1`
	}
	query += `
ORDER BY priority ASC, created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, accountID, typeKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WebhookSkill{}
	for rows.Next() {
		var sk domain.WebhookSkill
		if err := rows.Scan(&sk.ID, &sk.AccountID, &sk.TypeKey, &sk.SkillKey, &sk.SkillPrompt, &sk.MatchContains, &sk.ForcedAction, &sk.MemoryWriteMode, &sk.Priority, &sk.Enabled, &sk.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, nil
}

func (s *MySQLStore) UpdateWebhookSkill(ctx context.Context, skill domain.WebhookSkill) (domain.WebhookSkill, error) {
	if strings.TrimSpace(skill.ID) == "" {
		return domain.WebhookSkill{}, errors.New("skill id required")
	}
	if skill.Priority == 0 {
		skill.Priority = 100
	}
	if strings.TrimSpace(skill.MemoryWriteMode) == "" {
		skill.MemoryWriteMode = "update_or_insert"
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE webhook_skills
SET type_key=?, skill_key=?, skill_prompt=?, match_contains=?, forced_action=?, memory_write_mode=?, priority=?, enabled=?
WHERE id=? AND account_id=?`,
		skill.TypeKey, skill.SkillKey, skill.SkillPrompt, skill.MatchContains, skill.ForcedAction, skill.MemoryWriteMode, skill.Priority, skill.Enabled, skill.ID, skill.AccountID)
	if err != nil {
		return domain.WebhookSkill{}, err
	}
	var out domain.WebhookSkill
	err = s.db.QueryRowContext(ctx, `
SELECT id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at
FROM webhook_skills
WHERE id=? AND account_id=? LIMIT 1`, skill.ID, skill.AccountID).
		Scan(&out.ID, &out.AccountID, &out.TypeKey, &out.SkillKey, &out.SkillPrompt, &out.MatchContains, &out.ForcedAction, &out.MemoryWriteMode, &out.Priority, &out.Enabled, &out.CreatedAt)
	if err != nil {
		return domain.WebhookSkill{}, err
	}
	return out, nil
}

func (s *MySQLStore) UpsertAutoPromoteState(ctx context.Context, state domain.AutoPromoteState) (domain.AutoPromoteState, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO webhook_autopromote_states(account_id, type_key, status, validated_count, shadow_total, shadow_success, last_confidence, last_reason, updated_at, created_at)
VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE
  status=VALUES(status),
  validated_count=VALUES(validated_count),
  shadow_total=VALUES(shadow_total),
  shadow_success=VALUES(shadow_success),
  last_confidence=VALUES(last_confidence),
  last_reason=VALUES(last_reason),
  updated_at=UTC_TIMESTAMP()`,
		state.AccountID, state.TypeKey, state.Status, state.ValidatedCount, state.ShadowTotal, state.ShadowSuccess, state.LastConfidence, state.LastReason)
	if err != nil {
		return domain.AutoPromoteState{}, err
	}
	return s.GetAutoPromoteState(ctx, state.AccountID, state.TypeKey)
}

func (s *MySQLStore) GetAutoPromoteState(ctx context.Context, accountID, typeKey string) (domain.AutoPromoteState, error) {
	var st domain.AutoPromoteState
	err := s.db.QueryRowContext(ctx, `SELECT account_id, type_key, status, validated_count, shadow_total, shadow_success, last_confidence, last_reason, updated_at, created_at
FROM webhook_autopromote_states WHERE account_id=? AND type_key=? LIMIT 1`, accountID, typeKey).
		Scan(&st.AccountID, &st.TypeKey, &st.Status, &st.ValidatedCount, &st.ShadowTotal, &st.ShadowSuccess, &st.LastConfidence, &st.LastReason, &st.UpdatedAt, &st.CreatedAt)
	if err != nil {
		return domain.AutoPromoteState{}, err
	}
	return st, nil
}

func (s *MySQLStore) UpsertBYOKConfig(ctx context.Context, cfg domain.BYOKProviderConfig) (domain.BYOKProviderConfig, error) {
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO byok_provider_configs(id, account_id, provider, api_key, base_url, model, is_default, created_at)
		 VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP())
		 ON DUPLICATE KEY UPDATE api_key=VALUES(api_key), base_url=VALUES(base_url), model=VALUES(model), is_default=VALUES(is_default)`,
		cfg.ID, cfg.AccountID, cfg.Provider, cfg.APIKey, cfg.BaseURL, cfg.Model, cfg.IsDefault)
	if err != nil {
		return domain.BYOKProviderConfig{}, err
	}
	return s.GetBYOKConfig(ctx, cfg.AccountID, cfg.Provider)
}

func (s *MySQLStore) GetBYOKConfig(ctx context.Context, accountID, provider string) (domain.BYOKProviderConfig, error) {
	var cfg domain.BYOKProviderConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, provider, api_key, base_url, model, is_default, created_at FROM byok_provider_configs WHERE account_id=? AND provider=?`,
		accountID, provider).Scan(&cfg.ID, &cfg.AccountID, &cfg.Provider, &cfg.APIKey, &cfg.BaseURL, &cfg.Model, &cfg.IsDefault, &cfg.CreatedAt)
	if err != nil {
		return domain.BYOKProviderConfig{}, err
	}
	return cfg, nil
}

func (s *MySQLStore) GetDefaultBYOKConfig(ctx context.Context, accountID string) (domain.BYOKProviderConfig, error) {
	var cfg domain.BYOKProviderConfig
	err := s.db.QueryRowContext(ctx,
		`SELECT id, account_id, provider, api_key, base_url, model, is_default, created_at FROM byok_provider_configs WHERE account_id=? ORDER BY is_default DESC, created_at ASC LIMIT 1`,
		accountID).Scan(&cfg.ID, &cfg.AccountID, &cfg.Provider, &cfg.APIKey, &cfg.BaseURL, &cfg.Model, &cfg.IsDefault, &cfg.CreatedAt)
	if err != nil {
		return domain.BYOKProviderConfig{}, err
	}
	return cfg, nil
}

func (s *MySQLStore) ListBYOKConfigs(ctx context.Context, accountID string) ([]domain.BYOKProviderConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, account_id, provider, api_key, base_url, model, is_default, created_at FROM byok_provider_configs WHERE account_id=? ORDER BY is_default DESC, created_at ASC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BYOKProviderConfig
	for rows.Next() {
		var cfg domain.BYOKProviderConfig
		if err := rows.Scan(&cfg.ID, &cfg.AccountID, &cfg.Provider, &cfg.APIKey, &cfg.BaseURL, &cfg.Model, &cfg.IsDefault, &cfg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
