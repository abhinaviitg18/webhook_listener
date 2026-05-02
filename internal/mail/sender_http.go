package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type ResendSender struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type PostmarkSender struct {
	ServerToken string
	BaseURL     string
	HTTPClient  *http.Client
}

type ZeptoMailSender struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (s ResendSender) Send(ctx context.Context, mailbox Mailbox, request SendRequest, prior *Message) (string, string, error) {
	rawMessage, rfcMessageID, err := buildRawMessage(mailbox.EmailAddress, request, prior)
	if err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"from":    mailbox.EmailAddress,
		"to":      request.To,
		"cc":      request.CC,
		"bcc":     request.BCC,
		"subject": request.Subject,
		"text":    request.TextBody,
		"html":    request.HTMLBody,
	}
	if len(request.Attachments) > 0 {
		attachments := make([]map[string]any, 0, len(request.Attachments))
		for _, attachment := range request.Attachments {
			attachments = append(attachments, map[string]any{
				"filename": attachment.FileName,
				"content":  strings.TrimSpace(attachment.ContentBase64),
			})
		}
		payload["attachments"] = attachments
	}
	headers := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(s.APIKey),
	}
	if prior != nil && strings.TrimSpace(prior.RFCMessageID) != "" {
		headers["In-Reply-To"] = "<" + strings.Trim(prior.RFCMessageID, "<>") + ">"
	}
	body, status, err := doProviderJSONRequest(ctx, s.httpClient(), http.MethodPost, strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")+"/emails", payload, headers)
	if err != nil {
		return "", "", err
	}
	if status >= 300 {
		return "", "", sanitizeProviderError("resend", status, body)
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &resp)
	if strings.TrimSpace(resp.ID) == "" {
		resp.ID = strings.Trim(strings.TrimSpace(rfcMessageID), "<>")
	}
	_ = rawMessage
	return resp.ID, strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), nil
}

func (s PostmarkSender) Send(ctx context.Context, mailbox Mailbox, request SendRequest, prior *Message) (string, string, error) {
	rawMessage, rfcMessageID, err := buildRawMessage(mailbox.EmailAddress, request, prior)
	if err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"From":     mailbox.EmailAddress,
		"To":       strings.Join(request.To, ","),
		"Cc":       strings.Join(request.CC, ","),
		"Bcc":      strings.Join(request.BCC, ","),
		"Subject":  request.Subject,
		"TextBody": request.TextBody,
		"HtmlBody": request.HTMLBody,
	}
	if len(request.Attachments) > 0 {
		attachments := make([]map[string]any, 0, len(request.Attachments))
		for _, attachment := range request.Attachments {
			attachments = append(attachments, map[string]any{
				"Name":        attachment.FileName,
				"Content":     strings.TrimSpace(attachment.ContentBase64),
				"ContentType": strings.TrimSpace(attachment.ContentType),
				"ContentID":   strings.TrimSpace(attachment.ContentID),
			})
		}
		payload["Attachments"] = attachments
	}
	if prior != nil && strings.TrimSpace(prior.RFCMessageID) != "" {
		payload["Headers"] = []map[string]string{
			{"Name": "In-Reply-To", "Value": "<" + strings.Trim(prior.RFCMessageID, "<>") + ">"},
			{"Name": "References", "Value": strings.Join(buildReferenceHeader(prior), " ")},
		}
	}
	body, status, err := doProviderJSONRequest(ctx, s.httpClient(), http.MethodPost, strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")+"/email", payload, map[string]string{
		"X-Postmark-Server-Token": strings.TrimSpace(s.ServerToken),
		"Accept":                  "application/json",
	})
	if err != nil {
		return "", "", err
	}
	if status >= 300 {
		return "", "", sanitizeProviderError("postmark", status, body)
	}
	var resp struct {
		MessageID string `json:"MessageID"`
	}
	_ = json.Unmarshal([]byte(body), &resp)
	if strings.TrimSpace(resp.MessageID) == "" {
		resp.MessageID = strings.Trim(strings.TrimSpace(rfcMessageID), "<>")
	}
	_ = rawMessage
	return resp.MessageID, strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), nil
}

func (s ZeptoMailSender) Send(ctx context.Context, mailbox Mailbox, request SendRequest, prior *Message) (string, string, error) {
	rawMessage, rfcMessageID, err := buildRawMessage(mailbox.EmailAddress, request, prior)
	if err != nil {
		return "", "", err
	}
	payload := map[string]any{
		"from": map[string]string{
			"address": mailbox.EmailAddress,
		},
		"to":       buildZeptoRecipients(request.To),
		"subject":  request.Subject,
		"textbody": request.TextBody,
	}
	if strings.TrimSpace(request.HTMLBody) != "" {
		payload["htmlbody"] = request.HTMLBody
	}
	if len(request.CC) > 0 {
		payload["cc"] = buildZeptoRecipients(request.CC)
	}
	if len(request.BCC) > 0 {
		payload["bcc"] = buildZeptoRecipients(request.BCC)
	}
	headers := map[string]string{
		"Authorization": "Zoho-enczapikey " + strings.TrimSpace(s.APIKey),
	}
	if prior != nil && strings.TrimSpace(prior.RFCMessageID) != "" {
		headers["In-Reply-To"] = "<" + strings.Trim(prior.RFCMessageID, "<>") + ">"
	}
	body, status, err := doProviderJSONRequest(ctx, s.httpClient(), http.MethodPost, strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")+"/email", payload, headers)
	if err != nil {
		return "", "", err
	}
	if status >= 300 {
		return "", "", sanitizeProviderError("zeptomail", status, body)
	}
	var resp struct {
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &resp)
	messageID := strings.TrimSpace(resp.Data.MessageID)
	if messageID == "" {
		messageID = strings.Trim(strings.TrimSpace(rfcMessageID), "<>")
	}
	_ = rawMessage
	return messageID, strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), nil
}

func buildZeptoRecipients(addresses []string) []map[string]any {
	recipients := make([]map[string]any, 0, len(addresses))
	for _, address := range addresses {
		recipients = append(recipients, map[string]any{
			"email_address": map[string]string{
				"address": strings.TrimSpace(address),
			},
		})
	}
	return recipients
}

func doProviderJSONRequest(ctx context.Context, client *http.Client, method, url string, payload any, extraHeaders map[string]string) (string, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range extraHeaders {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return string(respBody), resp.StatusCode, nil
}

func (s ResendSender) httpClient() *http.Client    { return providerHTTPClient(s.HTTPClient) }
func (s PostmarkSender) httpClient() *http.Client  { return providerHTTPClient(s.HTTPClient) }
func (s ZeptoMailSender) httpClient() *http.Client { return providerHTTPClient(s.HTTPClient) }

func providerHTTPClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func buildReferenceHeader(prior *Message) []string {
	references := append([]string{}, prior.References...)
	references = append(references, strings.Trim(prior.RFCMessageID, "<>"))
	refs := make([]string, 0, len(references))
	for _, ref := range references {
		ref = strings.TrimSpace(strings.Trim(ref, "<>"))
		if ref != "" {
			refs = append(refs, "<"+ref+">")
		}
	}
	return refs
}
