package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agenthook.store/internal/domain"
	"agenthook.store/internal/webhookid"

	"github.com/google/uuid"
)

type AccountStore interface {
	GetAccountByPublicAlias(ctx context.Context, alias string) (domain.Account, error)
	ResolveSecretAnyType(ctx context.Context, accountID, secret string) (domain.WebhookSecret, error)
	GetWebhookTypeByID(ctx context.Context, typeID string) (domain.WebhookType, error)
	ListWebhookTypes(ctx context.Context, accountID string) ([]domain.WebhookType, error)
	ListSecrets(ctx context.Context, accountID, typeID string) ([]domain.WebhookSecret, error)
	GetAccount(ctx context.Context, id string) (domain.Account, error)
}

type Sender interface {
	Send(ctx context.Context, mailbox Mailbox, request SendRequest, prior *Message) (sesMessageID string, rfcMessageID string, err error)
}

type InboundFetcher interface {
	Fetch(ctx context.Context, bucket, key string) ([]byte, error)
}

type Config struct {
	MailDomain            string
	AgentHookBaseURL      string
	AgentHookOriginSecret string
	HTTPClient            *http.Client
}

type Service struct {
	AccountStore AccountStore
	Store        Store
	Sender       Sender
	Fetcher      InboundFetcher
	Config       Config
}

func (s *Service) SyncMailboxes(ctx context.Context, accountID string) ([]Mailbox, error) {
	acct, err := s.AccountStore.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	types, err := s.AccountStore.ListWebhookTypes(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, whType := range types {
		secrets, err := s.AccountStore.ListSecrets(ctx, accountID, whType.ID)
		if err != nil {
			return nil, err
		}
		for _, secret := range secrets {
			address := webhookid.BuildEmailAddress(acct.PublicAlias, secret.SecretValue, s.mailDomain())
			if _, err := s.Store.UpsertMailbox(ctx, Mailbox{
				AccountID:    accountID,
				TypeID:       whType.ID,
				SecretID:     secret.ID,
				PublicAlias:  acct.PublicAlias,
				EmailAddress: address,
				Status:       secret.Status,
			}); err != nil {
				return nil, err
			}
		}
	}
	return s.Store.ListMailboxes(ctx, accountID)
}

func (s *Service) IngestS3Object(ctx context.Context, bucket, key string) (Message, error) {
	if s.Fetcher == nil {
		return Message{}, errors.New("mail fetcher not configured")
	}
	raw, err := s.Fetcher.Fetch(ctx, bucket, key)
	if err != nil {
		return Message{}, err
	}
	return s.IngestRawMessage(ctx, raw, bucket, key)
}

func (s *Service) IngestRawMessage(ctx context.Context, raw []byte, bucket, key string) (Message, error) {
	parsed, err := ParseRawMessage(raw)
	if err != nil {
		return Message{}, err
	}
	if len(parsed.To) == 0 {
		return Message{}, errors.New("message missing recipient")
	}
	publicAlias, secret, domainName, ok := webhookid.ParseEmailAddress(extractAddress(parsed.To[0]))
	if !ok {
		return Message{}, errors.New("invalid mailbox address")
	}
	if domainName != strings.ToLower(s.mailDomain()) {
		return Message{}, errors.New("unsupported mail domain")
	}
	acct, err := s.AccountStore.GetAccountByPublicAlias(ctx, publicAlias)
	if err != nil {
		return Message{}, err
	}
	whSecret, err := s.AccountStore.ResolveSecretAnyType(ctx, acct.ID, secret)
	if err != nil {
		return Message{}, err
	}
	whType, err := s.AccountStore.GetWebhookTypeByID(ctx, whSecret.TypeID)
	if err != nil {
		return Message{}, err
	}
	mailboxAddress := webhookid.BuildEmailAddress(acct.PublicAlias, whSecret.SecretValue, s.mailDomain())
	mailbox, err := s.Store.UpsertMailbox(ctx, Mailbox{
		AccountID:    acct.ID,
		TypeID:       whType.ID,
		SecretID:     whSecret.ID,
		PublicAlias:  acct.PublicAlias,
		EmailAddress: mailboxAddress,
		Status:       whSecret.Status,
	})
	if err != nil {
		return Message{}, err
	}
	threadKey := canonicalThreadKey(parsed)
	thread, err := s.Store.UpsertThread(ctx, Thread{
		AccountID:        acct.ID,
		MailboxID:        mailbox.ID,
		ThreadKey:        threadKey,
		SubjectCanonical: canonicalSubject(parsed.Subject),
		LastMessageAt:    time.Now().UTC(),
	})
	if err != nil {
		return Message{}, err
	}
	attachments := make([]Attachment, 0, len(parsed.Attachments))
	for idx, attachment := range parsed.Attachments {
		attachments = append(attachments, Attachment{
			ID:          uuid.NewString(),
			FileName:    attachment.FileName,
			ContentType: attachment.ContentType,
			SizeBytes:   int64(len(attachment.Data)),
			S3Bucket:    bucket,
			S3Key:       fmt.Sprintf("%s.attachments.%d", key, idx),
			Inline:      attachment.Inline,
			ContentID:   attachment.ContentID,
		})
	}
	message, _, err := s.Store.CreateMessage(ctx, Message{
		AccountID:    acct.ID,
		MailboxID:    mailbox.ID,
		ThreadID:     thread.ID,
		Direction:    "inbound",
		SESMessageID: parsed.SESMessageID,
		RFCMessageID: parsed.RFCMessageID,
		InReplyTo:    parsed.InReplyTo,
		References:   parsed.References,
		From:         parsed.From,
		To:           parsed.To,
		CC:           parsed.CC,
		BCC:          parsed.BCC,
		Subject:      parsed.Subject,
		TextBody:     parsed.TextBody,
		HTMLBody:     parsed.HTMLBody,
		RawS3Bucket:  bucket,
		RawS3Key:     key,
		ParsedStatus: "parsed",
		ReceivedAt:   time.Now().UTC(),
	}, attachments)
	if err != nil {
		return Message{}, err
	}
	if err := s.forwardToAgentHook(ctx, acct.PublicAlias, whSecret.SecretValue, parsed, mailboxAddress, thread.ThreadKey); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) SendMessage(ctx context.Context, accountID, mailboxID string, request SendRequest) (Message, error) {
	if s.Sender == nil {
		return Message{}, errors.New("mail sender not configured")
	}
	mailbox, err := s.Store.GetMailbox(ctx, accountID, mailboxID)
	if err != nil {
		return Message{}, err
	}
	var prior *Message
	var threadID string
	if strings.TrimSpace(request.ReplyToMessageID) != "" {
		msg, _, err := s.Store.GetMessage(ctx, accountID, request.ReplyToMessageID)
		if err != nil {
			return Message{}, err
		}
		prior = &msg
		threadID = msg.ThreadID
	}
	if threadID == "" {
		thread, err := s.Store.UpsertThread(ctx, Thread{
			AccountID:        accountID,
			MailboxID:        mailboxID,
			ThreadKey:        uuid.NewString(),
			SubjectCanonical: canonicalSubject(request.Subject),
			LastMessageAt:    time.Now().UTC(),
		})
		if err != nil {
			return Message{}, err
		}
		threadID = thread.ID
	}
	sesMessageID, rfcMessageID, err := s.Sender.Send(ctx, mailbox, request, prior)
	if err != nil {
		return Message{}, err
	}
	attachments := make([]Attachment, 0, len(request.Attachments))
	for _, attachment := range request.Attachments {
		attachments = append(attachments, Attachment{
			FileName:    attachment.FileName,
			ContentType: attachment.ContentType,
			SizeBytes:   int64(len(attachment.ContentBase64)),
			Inline:      attachment.Inline,
			ContentID:   attachment.ContentID,
		})
	}
	message, _, err := s.Store.CreateMessage(ctx, Message{
		AccountID:    accountID,
		MailboxID:    mailboxID,
		ThreadID:     threadID,
		Direction:    "outbound",
		SESMessageID: sesMessageID,
		RFCMessageID: rfcMessageID,
		InReplyTo:    request.ReplyToMessageID,
		From:         []string{mailbox.EmailAddress},
		To:           request.To,
		CC:           request.CC,
		BCC:          request.BCC,
		Subject:      request.Subject,
		TextBody:     request.TextBody,
		HTMLBody:     request.HTMLBody,
		ParsedStatus: "sent",
		SentAt:       time.Now().UTC(),
	}, attachments)
	return message, err
}

func (s *Service) ReplyToMessage(ctx context.Context, accountID, messageID string, request SendRequest) (Message, error) {
	msg, _, err := s.Store.GetMessage(ctx, accountID, messageID)
	if err != nil {
		return Message{}, err
	}
	req := request
	req.ReplyToMessageID = messageID
	if len(req.To) == 0 {
		req.To = msg.From
	}
	if strings.TrimSpace(req.Subject) == "" {
		req.Subject = replySubject(msg.Subject)
	}
	return s.SendMessage(ctx, accountID, msg.MailboxID, req)
}

func (s *Service) forwardToAgentHook(ctx context.Context, publicAlias, secret string, parsed ParsedMessage, mailboxAddress, threadKey string) error {
	baseURL := strings.TrimRight(strings.TrimSpace(s.Config.AgentHookBaseURL), "/")
	if baseURL == "" {
		return nil
	}
	payload := map[string]any{
		"provider":            "ses-mail",
		"mailbox_address":     mailboxAddress,
		"from":                parsed.From,
		"to":                  parsed.To,
		"cc":                  parsed.CC,
		"bcc":                 parsed.BCC,
		"subject":             parsed.Subject,
		"text_body":           parsed.TextBody,
		"html_body":           parsed.HTMLBody,
		"html_body_present":   strings.TrimSpace(parsed.HTMLBody) != "",
		"thread_key":          threadKey,
		"in_reply_to":         parsed.InReplyTo,
		"references":          parsed.References,
		"ses_message_id":      parsed.SESMessageID,
		"rfc_message_id":      parsed.RFCMessageID,
		"attachment_count":    len(parsed.Attachments),
		"attachment_metadata": buildAttachmentMetadata(parsed.Attachments),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/"+publicAlias+"."+secret, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(s.Config.AgentHookOriginSecret) != "" {
		req.Header.Set("x-agenthook-origin-secret", s.Config.AgentHookOriginSecret)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("agenthook forward returned %s", resp.Status)
	}
	return nil
}

func (s *Service) httpClient() *http.Client {
	if s.Config.HTTPClient != nil {
		return s.Config.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Service) mailDomain() string {
	if strings.TrimSpace(s.Config.MailDomain) != "" {
		return strings.TrimSpace(s.Config.MailDomain)
	}
	return "app.agenthook.store"
}

func canonicalThreadKey(parsed ParsedMessage) string {
	switch {
	case parsed.InReplyTo != "":
		return parsed.InReplyTo
	case len(parsed.References) > 0:
		return parsed.References[len(parsed.References)-1]
	case parsed.RFCMessageID != "":
		return parsed.RFCMessageID
	default:
		return canonicalSubject(parsed.Subject)
	}
}

func canonicalSubject(subject string) string {
	normalized := strings.ToLower(strings.TrimSpace(subject))
	for _, prefix := range []string{"re:", "fwd:", "fw:"} {
		normalized = strings.TrimSpace(strings.TrimPrefix(normalized, prefix))
	}
	return normalized
}

func replySubject(subject string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		return strings.TrimSpace(subject)
	}
	return "Re: " + strings.TrimSpace(subject)
}

func extractAddress(value string) string {
	if strings.Contains(value, "<") && strings.Contains(value, ">") {
		start := strings.Index(value, "<")
		end := strings.Index(value[start:], ">")
		if end > 0 {
			return strings.TrimSpace(value[start+1 : start+end])
		}
	}
	return strings.TrimSpace(value)
}

func buildAttachmentMetadata(attachments []ParsedAttachment) []map[string]any {
	out := make([]map[string]any, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, map[string]any{
			"filename":     attachment.FileName,
			"content_type": attachment.ContentType,
			"size_bytes":   len(attachment.Data),
			"inline":       attachment.Inline,
			"content_id":   attachment.ContentID,
		})
	}
	return out
}
