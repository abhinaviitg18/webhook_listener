package mail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"

	"agenthook.store/internal/service"
)

func ParseRawMessage(raw []byte) (ParsedMessage, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ParsedMessage{}, err
	}
	header := msg.Header
	parsed := ParsedMessage{
		SESMessageID: firstHeader(header, "X-SES-Message-ID"),
		RFCMessageID: strings.TrimSpace(header.Get("Message-ID")),
		InReplyTo:    strings.TrimSpace(header.Get("In-Reply-To")),
		References:   splitReferences(header.Get("References")),
		From:         normalizeAddressList(header.Get("From")),
		To:           normalizeAddressList(header.Get("To")),
		CC:           normalizeAddressList(header.Get("Cc")),
		BCC:          normalizeAddressList(header.Get("Bcc")),
		Subject:      strings.TrimSpace(header.Get("Subject")),
	}
	contentType := header.Get("Content-Type")
	mediaType, params, _ := mime.ParseMediaType(contentType)
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return ParsedMessage{}, err
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		if err := parseMultipartInto(&parsed, mediaType, params["boundary"], bodyBytes); err != nil {
			return ParsedMessage{}, err
		}
	} else {
		assignDecodedBody(&parsed, mediaType, header.Get("Content-Transfer-Encoding"), bodyBytes)
	}
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = servicePayloadToText(parsed.HTMLBody)
	}
	return parsed, nil
}

func parseMultipartInto(parsed *ParsedMessage, mediaType, boundary string, body []byte) error {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		partBody, err := io.ReadAll(part)
		if err != nil {
			return err
		}
		partType, params, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if strings.HasPrefix(partType, "multipart/") {
			if err := parseMultipartInto(parsed, partType, params["boundary"], partBody); err != nil {
				return err
			}
			continue
		}
		if fileName := part.FileName(); fileName != "" || part.Header.Get("Content-Disposition") != "" && !strings.HasPrefix(partType, "text/") {
			data := decodePartBytes(part.Header.Get("Content-Transfer-Encoding"), partBody)
			parsed.Attachments = append(parsed.Attachments, ParsedAttachment{
				FileName:    fileName,
				ContentType: partType,
				ContentID:   strings.Trim(strings.TrimSpace(part.Header.Get("Content-ID")), "<>"),
				Inline:      strings.Contains(strings.ToLower(part.Header.Get("Content-Disposition")), "inline"),
				Data:        data,
			})
			continue
		}
		assignDecodedBody(parsed, partType, part.Header.Get("Content-Transfer-Encoding"), partBody)
	}
}

func assignDecodedBody(parsed *ParsedMessage, contentType, encoding string, body []byte) {
	decoded := string(decodePartBytes(encoding, body))
	switch {
	case strings.HasPrefix(strings.ToLower(contentType), "text/html"):
		if parsed.HTMLBody == "" {
			parsed.HTMLBody = strings.TrimSpace(decoded)
		}
	case strings.HasPrefix(strings.ToLower(contentType), "text/plain"), contentType == "":
		if parsed.TextBody == "" {
			parsed.TextBody = strings.TrimSpace(decoded)
		}
	}
}

func decodePartBytes(encoding string, body []byte) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(body)))
		if err == nil {
			return decoded
		}
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		if err == nil {
			return decoded
		}
	}
	return body
}

func normalizeAddressList(value string) []string {
	list, err := mail.ParseAddressList(value)
	if err != nil {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return []string{strings.TrimSpace(value)}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item == nil || strings.TrimSpace(item.Address) == "" {
			continue
		}
		if strings.TrimSpace(item.Name) != "" {
			out = append(out, item.Name+" <"+item.Address+">")
			continue
		}
		out = append(out, item.Address)
	}
	return out
}

func splitReferences(value string) []string {
	fields := strings.Fields(strings.TrimSpace(value))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		cleaned := strings.Trim(strings.TrimSpace(field), "<>")
		if cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func firstHeader(header mail.Header, key string) string {
	return strings.TrimSpace(header.Get(key))
}

func servicePayloadToText(value string) string {
	return strings.TrimSpace(service.SanitizeHTMLText(value))
}
