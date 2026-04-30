package mail

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

type MySQLStore struct {
	db *sql.DB
}

func NewMySQLStore(dsn string) (*MySQLStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	store := &MySQLStore{db: db}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *MySQLStore) ensureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS mailboxes (
			id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			type_id VARCHAR(64) NOT NULL,
			secret_id VARCHAR(64) NOT NULL,
			public_alias VARCHAR(128) NOT NULL,
			email_address VARCHAR(320) NOT NULL,
			status VARCHAR(32) NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE KEY uq_mailboxes_email (email_address),
			INDEX idx_mailboxes_account (account_id)
		)`,
		`CREATE TABLE IF NOT EXISTS mail_threads (
			id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			mailbox_id VARCHAR(64) NOT NULL,
			thread_key VARCHAR(255) NOT NULL,
			subject_canonical TEXT NOT NULL,
			last_message_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE KEY uq_mail_threads (account_id, mailbox_id, thread_key),
			INDEX idx_mail_threads_mailbox (mailbox_id, last_message_at)
		)`,
		`CREATE TABLE IF NOT EXISTS mail_messages (
			id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			mailbox_id VARCHAR(64) NOT NULL,
			thread_id VARCHAR(64) NOT NULL,
			direction VARCHAR(16) NOT NULL,
			ses_message_id VARCHAR(255) NULL,
			rfc_message_id VARCHAR(255) NULL,
			in_reply_to VARCHAR(255) NULL,
			references_json JSON NULL,
			from_json JSON NOT NULL,
			to_json JSON NOT NULL,
			cc_json JSON NULL,
			bcc_json JSON NULL,
			subject TEXT NOT NULL,
			text_body LONGTEXT NULL,
			html_body LONGTEXT NULL,
			raw_s3_bucket VARCHAR(255) NULL,
			raw_s3_key VARCHAR(1024) NULL,
			parsed_status VARCHAR(32) NOT NULL,
			received_at DATETIME NULL,
			sent_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			INDEX idx_mail_messages_mailbox (mailbox_id, created_at),
			INDEX idx_mail_messages_ses (ses_message_id),
			INDEX idx_mail_messages_rfc (rfc_message_id)
		)`,
		`CREATE TABLE IF NOT EXISTS mail_attachments (
			id VARCHAR(64) PRIMARY KEY,
			message_id VARCHAR(64) NOT NULL,
			filename VARCHAR(255) NOT NULL,
			content_type VARCHAR(255) NOT NULL,
			size_bytes BIGINT NOT NULL,
			s3_bucket VARCHAR(255) NULL,
			s3_key VARCHAR(1024) NULL,
			inline_flag BOOLEAN NOT NULL DEFAULT FALSE,
			content_id VARCHAR(255) NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_mail_attachments_message (message_id)
		)`,
		`CREATE TABLE IF NOT EXISTS mail_delivery_events (
			id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			message_id VARCHAR(64) NOT NULL,
			ses_event_type VARCHAR(64) NOT NULL,
			event_json JSON NOT NULL,
			occurred_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_mail_delivery_message (message_id, occurred_at)
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *MySQLStore) UpsertMailbox(ctx context.Context, mailbox Mailbox) (Mailbox, error) {
	if mailbox.ID == "" {
		mailbox.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if mailbox.CreatedAt.IsZero() {
		mailbox.CreatedAt = now
	}
	mailbox.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mailboxes(id, account_id, type_id, secret_id, public_alias, email_address, status, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
	type_id=VALUES(type_id),
	secret_id=VALUES(secret_id),
	public_alias=VALUES(public_alias),
	status=VALUES(status),
	updated_at=VALUES(updated_at)`,
		mailbox.ID, mailbox.AccountID, mailbox.TypeID, mailbox.SecretID, mailbox.PublicAlias, mailbox.EmailAddress, mailbox.Status, mailbox.CreatedAt, mailbox.UpdatedAt)
	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1062 {
			return s.FindMailboxByAddress(ctx, mailbox.EmailAddress)
		}
		return Mailbox{}, err
	}
	return s.FindMailboxByAddress(ctx, mailbox.EmailAddress)
}

func (s *MySQLStore) ListMailboxes(ctx context.Context, accountID string) ([]Mailbox, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, email_address, status, created_at, updated_at FROM mailboxes WHERE account_id=? ORDER BY email_address ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mailbox
	for rows.Next() {
		var mailbox Mailbox
		if err := rows.Scan(&mailbox.ID, &mailbox.AccountID, &mailbox.TypeID, &mailbox.SecretID, &mailbox.PublicAlias, &mailbox.EmailAddress, &mailbox.Status, &mailbox.CreatedAt, &mailbox.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, mailbox)
	}
	return out, nil
}

func (s *MySQLStore) GetMailbox(ctx context.Context, accountID, mailboxID string) (Mailbox, error) {
	var mailbox Mailbox
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, email_address, status, created_at, updated_at FROM mailboxes WHERE id=? AND account_id=? LIMIT 1`, mailboxID, accountID).
		Scan(&mailbox.ID, &mailbox.AccountID, &mailbox.TypeID, &mailbox.SecretID, &mailbox.PublicAlias, &mailbox.EmailAddress, &mailbox.Status, &mailbox.CreatedAt, &mailbox.UpdatedAt)
	if err != nil {
		return Mailbox{}, err
	}
	return mailbox, nil
}

func (s *MySQLStore) FindMailboxByAddress(ctx context.Context, emailAddress string) (Mailbox, error) {
	var mailbox Mailbox
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type_id, secret_id, public_alias, email_address, status, created_at, updated_at FROM mailboxes WHERE email_address=? LIMIT 1`, emailAddress).
		Scan(&mailbox.ID, &mailbox.AccountID, &mailbox.TypeID, &mailbox.SecretID, &mailbox.PublicAlias, &mailbox.EmailAddress, &mailbox.Status, &mailbox.CreatedAt, &mailbox.UpdatedAt)
	if err != nil {
		return Mailbox{}, err
	}
	return mailbox, nil
}

func (s *MySQLStore) UpsertThread(ctx context.Context, thread Thread) (Thread, error) {
	if thread.ID == "" {
		thread.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	thread.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mail_threads(id, account_id, mailbox_id, thread_key, subject_canonical, last_message_at, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
	subject_canonical=VALUES(subject_canonical),
	last_message_at=VALUES(last_message_at),
	updated_at=VALUES(updated_at)`,
		thread.ID, thread.AccountID, thread.MailboxID, thread.ThreadKey, thread.SubjectCanonical, thread.LastMessageAt, thread.CreatedAt, thread.UpdatedAt)
	if err != nil {
		return Thread{}, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT id, account_id, mailbox_id, thread_key, subject_canonical, last_message_at, created_at, updated_at FROM mail_threads WHERE account_id=? AND mailbox_id=? AND thread_key=? LIMIT 1`, thread.AccountID, thread.MailboxID, thread.ThreadKey).
		Scan(&thread.ID, &thread.AccountID, &thread.MailboxID, &thread.ThreadKey, &thread.SubjectCanonical, &thread.LastMessageAt, &thread.CreatedAt, &thread.UpdatedAt)
	return thread, err
}

func (s *MySQLStore) CreateMessage(ctx context.Context, message Message, attachments []Attachment) (Message, []Attachment, error) {
	if message.ID == "" {
		message.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	message.UpdatedAt = now
	referencesJSON, _ := json.Marshal(message.References)
	fromJSON, _ := json.Marshal(message.From)
	toJSON, _ := json.Marshal(message.To)
	ccJSON, _ := json.Marshal(message.CC)
	bccJSON, _ := json.Marshal(message.BCC)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mail_messages(id, account_id, mailbox_id, thread_id, direction, ses_message_id, rfc_message_id, in_reply_to, references_json, from_json, to_json, cc_json, bcc_json, subject, text_body, html_body, raw_s3_bucket, raw_s3_key, parsed_status, received_at, sent_at, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		message.ID, message.AccountID, message.MailboxID, message.ThreadID, message.Direction, nullIfEmpty(message.SESMessageID), nullIfEmpty(message.RFCMessageID), nullIfEmpty(message.InReplyTo), nullableJSON(referencesJSON), string(fromJSON), string(toJSON), nullableJSON(ccJSON), nullableJSON(bccJSON), message.Subject, nullIfEmpty(message.TextBody), nullIfEmpty(message.HTMLBody), nullIfEmpty(message.RawS3Bucket), nullIfEmpty(message.RawS3Key), message.ParsedStatus, nullableTime(message.ReceivedAt), nullableTime(message.SentAt), message.CreatedAt, message.UpdatedAt)
	if err != nil {
		return Message{}, nil, err
	}
	outAttachments := make([]Attachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.ID == "" {
			attachment.ID = uuid.NewString()
		}
		attachment.MessageID = message.ID
		if attachment.CreatedAt.IsZero() {
			attachment.CreatedAt = now
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO mail_attachments(id, message_id, filename, content_type, size_bytes, s3_bucket, s3_key, inline_flag, content_id, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			attachment.ID, attachment.MessageID, attachment.FileName, attachment.ContentType, attachment.SizeBytes, nullIfEmpty(attachment.S3Bucket), nullIfEmpty(attachment.S3Key), attachment.Inline, nullIfEmpty(attachment.ContentID), attachment.CreatedAt)
		if err != nil {
			return Message{}, nil, err
		}
		outAttachments = append(outAttachments, attachment)
	}
	return message, outAttachments, nil
}

func (s *MySQLStore) ListMessages(ctx context.Context, accountID, mailboxID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, mailbox_id, thread_id, direction, COALESCE(ses_message_id,''), COALESCE(rfc_message_id,''), COALESCE(in_reply_to,''), COALESCE(references_json, '[]'), from_json, to_json, COALESCE(cc_json,'[]'), COALESCE(bcc_json,'[]'), subject, COALESCE(text_body,''), COALESCE(html_body,''), COALESCE(raw_s3_bucket,''), COALESCE(raw_s3_key,''), parsed_status, received_at, sent_at, created_at, updated_at FROM mail_messages WHERE account_id=? AND mailbox_id=? ORDER BY created_at DESC LIMIT ?`, accountID, mailboxID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var message Message
		var referencesJSON, fromJSON, toJSON, ccJSON, bccJSON string
		var receivedAt, sentAt sql.NullTime
		if err := rows.Scan(&message.ID, &message.AccountID, &message.MailboxID, &message.ThreadID, &message.Direction, &message.SESMessageID, &message.RFCMessageID, &message.InReplyTo, &referencesJSON, &fromJSON, &toJSON, &ccJSON, &bccJSON, &message.Subject, &message.TextBody, &message.HTMLBody, &message.RawS3Bucket, &message.RawS3Key, &message.ParsedStatus, &receivedAt, &sentAt, &message.CreatedAt, &message.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(referencesJSON), &message.References)
		_ = json.Unmarshal([]byte(fromJSON), &message.From)
		_ = json.Unmarshal([]byte(toJSON), &message.To)
		_ = json.Unmarshal([]byte(ccJSON), &message.CC)
		_ = json.Unmarshal([]byte(bccJSON), &message.BCC)
		if receivedAt.Valid {
			message.ReceivedAt = receivedAt.Time
		}
		if sentAt.Valid {
			message.SentAt = sentAt.Time
		}
		out = append(out, message)
	}
	return out, nil
}

func (s *MySQLStore) GetMessage(ctx context.Context, accountID, messageID string) (Message, []Attachment, error) {
	var message Message
	var referencesJSON, fromJSON, toJSON, ccJSON, bccJSON string
	var receivedAt, sentAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, mailbox_id, thread_id, direction, COALESCE(ses_message_id,''), COALESCE(rfc_message_id,''), COALESCE(in_reply_to,''), COALESCE(references_json, '[]'), from_json, to_json, COALESCE(cc_json,'[]'), COALESCE(bcc_json,'[]'), subject, COALESCE(text_body,''), COALESCE(html_body,''), COALESCE(raw_s3_bucket,''), COALESCE(raw_s3_key,''), parsed_status, received_at, sent_at, created_at, updated_at FROM mail_messages WHERE id=? AND account_id=? LIMIT 1`, messageID, accountID).
		Scan(&message.ID, &message.AccountID, &message.MailboxID, &message.ThreadID, &message.Direction, &message.SESMessageID, &message.RFCMessageID, &message.InReplyTo, &referencesJSON, &fromJSON, &toJSON, &ccJSON, &bccJSON, &message.Subject, &message.TextBody, &message.HTMLBody, &message.RawS3Bucket, &message.RawS3Key, &message.ParsedStatus, &receivedAt, &sentAt, &message.CreatedAt, &message.UpdatedAt)
	if err != nil {
		return Message{}, nil, err
	}
	_ = json.Unmarshal([]byte(referencesJSON), &message.References)
	_ = json.Unmarshal([]byte(fromJSON), &message.From)
	_ = json.Unmarshal([]byte(toJSON), &message.To)
	_ = json.Unmarshal([]byte(ccJSON), &message.CC)
	_ = json.Unmarshal([]byte(bccJSON), &message.BCC)
	if receivedAt.Valid {
		message.ReceivedAt = receivedAt.Time
	}
	if sentAt.Valid {
		message.SentAt = sentAt.Time
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, message_id, filename, content_type, size_bytes, COALESCE(s3_bucket,''), COALESCE(s3_key,''), inline_flag, COALESCE(content_id,''), created_at FROM mail_attachments WHERE message_id=? ORDER BY created_at ASC`, messageID)
	if err != nil {
		return Message{}, nil, err
	}
	defer rows.Close()
	var attachments []Attachment
	for rows.Next() {
		var attachment Attachment
		if err := rows.Scan(&attachment.ID, &attachment.MessageID, &attachment.FileName, &attachment.ContentType, &attachment.SizeBytes, &attachment.S3Bucket, &attachment.S3Key, &attachment.Inline, &attachment.ContentID, &attachment.CreatedAt); err != nil {
			return Message{}, nil, err
		}
		attachments = append(attachments, attachment)
	}
	return message, attachments, nil
}

func (s *MySQLStore) RecordDeliveryEvent(ctx context.Context, event DeliveryEvent) (DeliveryEvent, error) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO mail_delivery_events(id, account_id, message_id, ses_event_type, event_json, occurred_at, created_at) VALUES(?,?,?,?,?,?,?)`,
		event.ID, event.AccountID, event.MessageID, event.SESEventType, event.EventJSON, event.OccurredAt, event.CreatedAt)
	if err != nil {
		return DeliveryEvent{}, err
	}
	return event, nil
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 || string(raw) == "[]" {
		return nil
	}
	return string(raw)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
