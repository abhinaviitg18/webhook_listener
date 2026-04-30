package mail

import "time"

type Mailbox struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	TypeID       string    `json:"type_id"`
	SecretID     string    `json:"secret_id"`
	PublicAlias  string    `json:"public_alias"`
	EmailAddress string    `json:"email_address"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Thread struct {
	ID               string    `json:"id"`
	AccountID        string    `json:"account_id"`
	MailboxID        string    `json:"mailbox_id"`
	ThreadKey        string    `json:"thread_key"`
	SubjectCanonical string    `json:"subject_canonical"`
	LastMessageAt    time.Time `json:"last_message_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Message struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	MailboxID    string    `json:"mailbox_id"`
	ThreadID     string    `json:"thread_id"`
	Direction    string    `json:"direction"`
	SESMessageID string    `json:"ses_message_id,omitempty"`
	RFCMessageID string    `json:"rfc_message_id,omitempty"`
	InReplyTo    string    `json:"in_reply_to,omitempty"`
	References   []string  `json:"references,omitempty"`
	From         []string  `json:"from,omitempty"`
	To           []string  `json:"to,omitempty"`
	CC           []string  `json:"cc,omitempty"`
	BCC          []string  `json:"bcc,omitempty"`
	Subject      string    `json:"subject"`
	TextBody     string    `json:"text_body"`
	HTMLBody     string    `json:"html_body,omitempty"`
	RawS3Bucket  string    `json:"raw_s3_bucket,omitempty"`
	RawS3Key     string    `json:"raw_s3_key,omitempty"`
	ParsedStatus string    `json:"parsed_status"`
	ReceivedAt   time.Time `json:"received_at,omitempty"`
	SentAt       time.Time `json:"sent_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Attachment struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id"`
	FileName    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	S3Bucket    string    `json:"s3_bucket,omitempty"`
	S3Key       string    `json:"s3_key,omitempty"`
	Inline      bool      `json:"inline"`
	ContentID   string    `json:"content_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type DeliveryEvent struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	MessageID    string    `json:"message_id"`
	SESEventType string    `json:"ses_event_type"`
	EventJSON    string    `json:"event_json"`
	OccurredAt   time.Time `json:"occurred_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type ParsedMessage struct {
	SESMessageID string
	RFCMessageID string
	InReplyTo    string
	References   []string
	From         []string
	To           []string
	CC           []string
	BCC          []string
	Subject      string
	TextBody     string
	HTMLBody     string
	Attachments  []ParsedAttachment
}

type ParsedAttachment struct {
	FileName    string
	ContentType string
	ContentID   string
	Inline      bool
	Data        []byte
}

type SendRequest struct {
	FromMailboxID    string               `json:"from_mailbox_id,omitempty"`
	To               []string             `json:"to"`
	CC               []string             `json:"cc,omitempty"`
	BCC              []string             `json:"bcc,omitempty"`
	Subject          string               `json:"subject"`
	TextBody         string               `json:"text_body"`
	HTMLBody         string               `json:"html_body,omitempty"`
	Attachments      []OutgoingAttachment `json:"attachments,omitempty"`
	ReplyToMessageID string               `json:"reply_to_message_id,omitempty"`
}

type OutgoingAttachment struct {
	FileName      string `json:"filename"`
	ContentType   string `json:"content_type"`
	ContentBase64 string `json:"content_base64"`
	Inline        bool   `json:"inline,omitempty"`
	ContentID     string `json:"content_id,omitempty"`
}
