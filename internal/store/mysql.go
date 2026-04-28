package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/security"
)

type MySQLStore struct {
	db                *sql.DB
	hasRawPayloadJSON bool
	hasProcessedText  bool
	schemaCheckOnce   sync.Once
}

func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
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

func (s *MySQLStore) ensureEventSchemaCapabilities() {
	s.schemaCheckOnce.Do(func() {
		s.hasRawPayloadJSON = s.columnExists("webhook_events", "raw_payload_json")
		s.hasProcessedText = s.columnExists("webhook_events", "processed_text")
	})
}

func (s *MySQLStore) CreateAccount(ctx context.Context, email string) (domain.Account, string, error) {
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
	_, ierr := s.db.ExecContext(ctx, `INSERT INTO accounts(id, slug, owner_email, created_at) VALUES(?,?,?,UTC_TIMESTAMP())`, id, slug, email)
	if ierr != nil {
		return domain.Account{}, "", ierr
	}
	_, ierr = s.db.ExecContext(ctx, `INSERT INTO account_tokens(id, account_id, token_hash, created_at) VALUES(?,?,?,UTC_TIMESTAMP())`, uuid.NewString(), id, hash)
	if ierr != nil {
		return domain.Account{}, "", ierr
	}
	return domain.Account{ID: id, Slug: slug, OwnerEmail: email, TokenHash: hash, CreatedAt: time.Now().UTC()}, token, nil
}

func (s *MySQLStore) GetAccountBySlug(ctx context.Context, slug string) (domain.Account, error) {
	var a domain.Account
	err := s.db.QueryRowContext(ctx, `SELECT id, slug, owner_email, created_at FROM accounts WHERE slug=? LIMIT 1`, slug).Scan(&a.ID, &a.Slug, &a.OwnerEmail, &a.CreatedAt)
	if err != nil {
		return domain.Account{}, err
	}
	return a, nil
}

func (s *MySQLStore) GetAccountByToken(ctx context.Context, token string) (domain.Account, error) {
	h := security.HashValue(token)
	var a domain.Account
	err := s.db.QueryRowContext(ctx, `
SELECT a.id, a.slug, a.owner_email, a.created_at
FROM accounts a
JOIN account_tokens t ON t.account_id=a.id
WHERE t.token_hash=? AND t.revoked_at IS NULL
ORDER BY t.created_at DESC
LIMIT 1`, h).Scan(&a.ID, &a.Slug, &a.OwnerEmail, &a.CreatedAt)
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
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_types(id, account_id, type_key, plain_text_action, use_llm_fallback, created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP())`, id, accountID, typeKey, plainTextAction, useLLMFallback)
	if err != nil {
		return domain.WebhookType{}, err
	}
	return domain.WebhookType{ID: id, AccountID: accountID, TypeKey: typeKey, PlainTextAction: plainTextAction, UseLLMFallback: useLLMFallback, CreatedAt: time.Now().UTC()}, nil
}

func (s *MySQLStore) GetAccount(ctx context.Context, id string) (domain.Account, error) {
	var a domain.Account
	err := s.db.QueryRowContext(ctx, "SELECT id, slug, owner_email, created_at FROM accounts WHERE id=?", id).
		Scan(&a.ID, &a.Slug, &a.OwnerEmail, &a.CreatedAt)
	return a, err
}

func (s *MySQLStore) ListWebhookTypes(ctx context.Context, accountID string) ([]domain.WebhookType, error) {
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
	var w domain.WebhookType
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_key, plain_text_action, use_llm_fallback, created_at FROM webhook_types WHERE account_id=? AND type_key=? LIMIT 1`, accountID, typeKey).Scan(&w.ID, &w.AccountID, &w.TypeKey, &w.PlainTextAction, &w.UseLLMFallback, &w.CreatedAt)
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM webhook_types WHERE id=? AND account_id=?`, typeID, accountID)
	return err
}

func (s *MySQLStore) CreateSecret(ctx context.Context, accountID, typeID string) (domain.WebhookSecret, string, error) {
	raw, err := security.NewToken(18)
	if err != nil {
		return domain.WebhookSecret{}, "", err
	}
	id := uuid.NewString()
	_, err = s.db.ExecContext(ctx, `INSERT INTO webhook_secrets(id, account_id, type_id, secret_value, status, created_at) VALUES(?,?,?,?, 'active', UTC_TIMESTAMP())`, id, accountID, typeID, raw)
	if err != nil {
		return domain.WebhookSecret{}, "", err
	}
	return domain.WebhookSecret{ID: id, AccountID: accountID, TypeID: typeID, SecretValue: raw, Status: "active", CreatedAt: time.Now().UTC()}, raw, nil
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
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_secrets SET status='revoked' WHERE id=? AND account_id=?`, secretID, accountID)
	return err
}

func (s *MySQLStore) ValidateSecret(ctx context.Context, accountID, typeID, secret string) (domain.WebhookSecret, error) {
	var sec domain.WebhookSecret
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_id, COALESCE(secret_value, ''), status, created_at FROM webhook_secrets WHERE account_id=? AND type_id=? AND secret_value=? AND status='active' LIMIT 1`, accountID, typeID, secret).Scan(&sec.ID, &sec.AccountID, &sec.TypeID, &sec.SecretValue, &sec.Status, &sec.CreatedAt)
	if err != nil {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
}

func (s *MySQLStore) ResolveSecretAnyType(ctx context.Context, accountID, secret string) (domain.WebhookSecret, error) {
	var sec domain.WebhookSecret
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_id, COALESCE(secret_value, ''), status, created_at FROM webhook_secrets WHERE account_id=? AND secret_value=? AND status='active' LIMIT 1`, accountID, secret).Scan(&sec.ID, &sec.AccountID, &sec.TypeID, &sec.SecretValue, &sec.Status, &sec.CreatedAt)
	if err != nil {
		return domain.WebhookSecret{}, errors.New("invalid secret")
	}
	return sec, nil
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

func (s *MySQLStore) CreateEvent(ctx context.Context, e domain.WebhookEvent) (domain.WebhookEvent, error) {
	s.ensureEventSchemaCapabilities()
	e.ID = uuid.NewString()
	e.CreatedAt = time.Now().UTC()
	query := `INSERT INTO webhook_events(id, account_id, type_id, secret_id, request_id, source_event_id, type_key, payload_json, action_selected, status, created_at) VALUES(?,?,?,?,?,?,?,?,?, ?,UTC_TIMESTAMP())`
	args := []interface{}{e.ID, e.AccountID, e.TypeID, e.SecretID, e.RequestID, nullIfEmpty(e.SourceEventID), e.TypeKey, e.PayloadJSON, e.ActionSelected, e.Status}
	if s.hasRawPayloadJSON && s.hasProcessedText {
		query = `INSERT INTO webhook_events(id, account_id, type_id, secret_id, request_id, source_event_id, type_key, raw_payload_json, payload_json, processed_text, action_selected, status, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?, ?,UTC_TIMESTAMP())`
		args = []interface{}{e.ID, e.AccountID, e.TypeID, e.SecretID, e.RequestID, nullIfEmpty(e.SourceEventID), e.TypeKey, nullIfEmpty(e.RawPayloadJSON), e.PayloadJSON, nullIfEmpty(e.ProcessedText), e.ActionSelected, e.Status}
	}
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return domain.WebhookEvent{}, err
	}
	return e, nil
}

func (s *MySQLStore) UpdateEventStatus(ctx context.Context, eventID, status, action string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_events SET status=?, action_selected=? WHERE id=?`, status, action, eventID)
	return err
}

func (s *MySQLStore) ListEvents(ctx context.Context, accountID string, limit int) ([]domain.WebhookEvent, error) {
	s.ensureEventSchemaCapabilities()
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, '', payload_json, '', action_selected, '', status, created_at FROM webhook_events WHERE account_id=? ORDER BY created_at DESC LIMIT ?`
	if s.hasRawPayloadJSON && s.hasProcessedText {
		query = `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, COALESCE(raw_payload_json,''), payload_json, COALESCE(processed_text,''), action_selected, COALESCE(tags_json,''), status, created_at FROM webhook_events WHERE account_id=? ORDER BY created_at DESC LIMIT ?`
	}
	rows, err := s.db.QueryContext(ctx, query, accountID, limit)
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
	s.ensureEventSchemaCapabilities()
	query := `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, '', payload_json, '', action_selected, '', status, created_at FROM webhook_events WHERE account_id=? AND id=?`
	if s.hasRawPayloadJSON && s.hasProcessedText {
		query = `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, COALESCE(raw_payload_json,''), payload_json, COALESCE(processed_text,''), action_selected, COALESCE(tags_json,''), status, created_at FROM webhook_events WHERE account_id=? AND id=?`
	}
	var e domain.WebhookEvent
	err := s.db.QueryRowContext(ctx, query, accountID, eventID).
		Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.TagsJSON, &e.Status, &e.CreatedAt)
	return e, err
}

func (s *MySQLStore) FindEventBySourceEventID(ctx context.Context, accountID, sourceEventID string) (domain.WebhookEvent, error) {
	s.ensureEventSchemaCapabilities()
	if strings.TrimSpace(sourceEventID) == "" {
		return domain.WebhookEvent{}, errors.New("source event id required")
	}
	var e domain.WebhookEvent
	query := `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, '', payload_json, '', action_selected, status, created_at FROM webhook_events WHERE account_id=? AND source_event_id=? LIMIT 1`
	if s.hasRawPayloadJSON && s.hasProcessedText {
		query = `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, COALESCE(raw_payload_json,''), payload_json, COALESCE(processed_text,''), action_selected, status, created_at FROM webhook_events WHERE account_id=? AND source_event_id=? LIMIT 1`
	}
	err := s.db.QueryRowContext(ctx, query, accountID, sourceEventID).
		Scan(&e.ID, &e.AccountID, &e.TypeID, &e.SecretID, &e.RequestID, &e.SourceEventID, &e.TypeKey, &e.RawPayloadJSON, &e.PayloadJSON, &e.ProcessedText, &e.ActionSelected, &e.Status, &e.CreatedAt)
	if err != nil {
		return domain.WebhookEvent{}, err
	}
	return e, nil
}

func (s *MySQLStore) ListEventsByTag(ctx context.Context, accountID, tag string, limit int) ([]domain.WebhookEvent, error) {
	s.ensureEventSchemaCapabilities()
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, account_id, type_id, secret_id, request_id, COALESCE(source_event_id,''), type_key, COALESCE(raw_payload_json,''), payload_json, COALESCE(processed_text,''), action_selected, COALESCE(tags_json,''), status, created_at FROM webhook_events WHERE account_id=? AND tags_json LIKE ? ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, accountID, `%"`+tag+`"%`, limit)
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
	return err
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
	rows, err := s.db.QueryContext(ctx, `
SELECT id, account_id, type_key, skill_key, skill_prompt, match_contains, forced_action, memory_write_mode, priority, enabled, created_at
FROM webhook_skills
WHERE account_id=? AND type_key=? AND enabled=1
ORDER BY priority ASC, created_at ASC`, accountID, typeKey)
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
