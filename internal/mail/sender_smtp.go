package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

type smtpSendClient interface {
	Send(addr string, auth smtp.Auth, from string, to []string, msg []byte, useTLS bool, host string) error
}

type defaultSMTPSendClient struct{}

type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	UseTLS   bool
	Client   smtpSendClient
}

func (s SMTPSender) Send(_ context.Context, mailbox Mailbox, request SendRequest, prior *Message) (string, string, error) {
	host := strings.TrimSpace(s.Host)
	if host == "" || s.Port <= 0 {
		return "", "", fmt.Errorf("smtp client not configured")
	}
	rawMessage, rfcMessageID, err := buildRawMessage(mailbox.EmailAddress, request, prior)
	if err != nil {
		return "", "", err
	}
	var auth smtp.Auth
	if strings.TrimSpace(s.Username) != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, host)
	}
	client := s.Client
	if client == nil {
		client = defaultSMTPSendClient{}
	}
	addr := net.JoinHostPort(host, strconv.Itoa(s.Port))
	recipients := append([]string{}, request.To...)
	recipients = append(recipients, request.CC...)
	recipients = append(recipients, request.BCC...)
	if err := client.Send(addr, auth, mailbox.EmailAddress, recipients, rawMessage, s.UseTLS, host); err != nil {
		return "", "", err
	}
	return strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), strings.Trim(strings.TrimSpace(rfcMessageID), "<>"), nil
}

func (defaultSMTPSendClient) Send(addr string, auth smtp.Auth, from string, to []string, msg []byte, useTLS bool, host string) error {
	if !useTLS {
		return smtp.SendMail(addr, auth, from, to, msg)
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Quit()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return err
	}
	return wc.Close()
}
