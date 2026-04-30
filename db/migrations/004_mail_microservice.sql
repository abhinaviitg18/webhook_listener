CREATE TABLE IF NOT EXISTS mailboxes (
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
);

CREATE TABLE IF NOT EXISTS mail_threads (
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
);

CREATE TABLE IF NOT EXISTS mail_messages (
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
);

CREATE TABLE IF NOT EXISTS mail_attachments (
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
);

CREATE TABLE IF NOT EXISTS mail_delivery_events (
  id VARCHAR(64) PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  message_id VARCHAR(64) NOT NULL,
  ses_event_type VARCHAR(64) NOT NULL,
  event_json JSON NOT NULL,
  occurred_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  INDEX idx_mail_delivery_message (message_id, occurred_at)
);
