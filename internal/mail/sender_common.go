package mail

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
)

func buildRawMessage(from string, request SendRequest, prior *Message) ([]byte, string, error) {
	hostPart := "app.agenthook.store"
	if _, host, found := strings.Cut(from, "@"); found && strings.TrimSpace(host) != "" {
		hostPart = strings.TrimSpace(host)
	}
	rfcMessageID := "<" + generatedMessageIDLocalPart(from, request.Subject) + "@" + hostPart + ">"

	var raw bytes.Buffer
	fmt.Fprintf(&raw, "From: %s\r\n", from)
	fmt.Fprintf(&raw, "To: %s\r\n", strings.Join(request.To, ", "))
	if len(request.CC) > 0 {
		fmt.Fprintf(&raw, "Cc: %s\r\n", strings.Join(request.CC, ", "))
	}
	fmt.Fprintf(&raw, "Subject: %s\r\n", request.Subject)
	fmt.Fprintf(&raw, "Message-ID: %s\r\n", rfcMessageID)
	fmt.Fprintf(&raw, "MIME-Version: 1.0\r\n")
	if prior != nil && strings.TrimSpace(prior.RFCMessageID) != "" {
		fmt.Fprintf(&raw, "In-Reply-To: <%s>\r\n", strings.Trim(prior.RFCMessageID, "<>"))
		references := append([]string{}, prior.References...)
		references = append(references, strings.Trim(prior.RFCMessageID, "<>"))
		if len(references) > 0 {
			var refs []string
			for _, ref := range references {
				ref = strings.TrimSpace(strings.Trim(ref, "<>"))
				if ref != "" {
					refs = append(refs, "<"+ref+">")
				}
			}
			if len(refs) > 0 {
				fmt.Fprintf(&raw, "References: %s\r\n", strings.Join(refs, " "))
			}
		}
	}

	mixed := multipart.NewWriter(&raw)
	fmt.Fprintf(&raw, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", mixed.Boundary())

	altHeader := textproto.MIMEHeader{}
	altHeader.Set("Content-Type", "multipart/alternative; boundary=ALT")
	altPart, err := mixed.CreatePart(altHeader)
	if err != nil {
		return nil, "", err
	}
	fmt.Fprintf(altPart, "--ALT\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", request.TextBody)
	if strings.TrimSpace(request.HTMLBody) != "" {
		fmt.Fprintf(altPart, "--ALT\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n", request.HTMLBody)
	}
	fmt.Fprintf(altPart, "--ALT--\r\n")

	for _, attachment := range request.Attachments {
		partHeader := textproto.MIMEHeader{}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		partHeader.Set("Content-Type", contentType)
		disposition := "attachment"
		if attachment.Inline {
			disposition = "inline"
		}
		partHeader.Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, attachment.FileName))
		partHeader.Set("Content-Transfer-Encoding", "base64")
		if strings.TrimSpace(attachment.ContentID) != "" {
			partHeader.Set("Content-ID", "<"+strings.TrimSpace(attachment.ContentID)+">")
		}
		part, err := mixed.CreatePart(partHeader)
		if err != nil {
			return nil, "", err
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(attachment.ContentBase64))
		if err != nil {
			return nil, "", err
		}
		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(decoded)))
		base64.StdEncoding.Encode(encoded, decoded)
		for len(encoded) > 76 {
			if _, err := part.Write(encoded[:76]); err != nil {
				return nil, "", err
			}
			if _, err := part.Write([]byte("\r\n")); err != nil {
				return nil, "", err
			}
			encoded = encoded[76:]
		}
		if len(encoded) > 0 {
			if _, err := part.Write(encoded); err != nil {
				return nil, "", err
			}
			if _, err := part.Write([]byte("\r\n")); err != nil {
				return nil, "", err
			}
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, "", err
	}
	return raw.Bytes(), rfcMessageID, nil
}

func generatedMessageIDLocalPart(from, subject string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(from + "|" + subject))))
	encoded := base64.RawURLEncoding.EncodeToString(sum[:])
	if len(encoded) > 24 {
		return strings.ToLower(encoded[:24])
	}
	return strings.ToLower(encoded)
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func sanitizeProviderError(provider string, status int, body string) error {
	body = strings.TrimSpace(body)
	if len(body) > 240 {
		body = body[:240]
	}
	if body == "" {
		return fmt.Errorf("%s send failed with status %d", provider, status)
	}
	return fmt.Errorf("%s send failed with status %d: %s", provider, status, body)
}
