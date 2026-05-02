package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type SESSender struct {
	Client *sesv2.Client
}

func (s SESSender) Send(ctx context.Context, mailbox Mailbox, request SendRequest, prior *Message) (string, string, error) {
	if s.Client == nil {
		return "", "", fmt.Errorf("ses client not configured")
	}
	rawMessage, rfcMessageID, err := buildRawMessage(mailbox.EmailAddress, request, prior)
	if err != nil {
		return "", "", err
	}
	resp, err := s.Client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: &mailbox.EmailAddress,
		Destination: &types.Destination{
			ToAddresses:  request.To,
			CcAddresses:  request.CC,
			BccAddresses: request.BCC,
		},
		Content: &types.EmailContent{
			Raw: &types.RawMessage{Data: rawMessage},
		},
	})
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(valueOrEmpty(resp.MessageId)), strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), nil
}
