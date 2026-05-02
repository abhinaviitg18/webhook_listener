package mail

import (
	"context"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"testing"
)

type stubSMTPClient struct {
	addr    string
	from    string
	to      []string
	msg     []byte
	useTLS  bool
	host    string
	sendErr error
}

func (s *stubSMTPClient) Send(addr string, _ smtp.Auth, from string, to []string, msg []byte, useTLS bool, host string) error {
	s.addr = addr
	s.from = from
	s.to = append([]string{}, to...)
	s.msg = append([]byte{}, msg...)
	s.useTLS = useTLS
	s.host = host
	return s.sendErr
}

func TestSMTPSenderSend(t *testing.T) {
	t.Parallel()

	stub := &stubSMTPClient{}
	sender := SMTPSender{
		Host:   "smtp.example.com",
		Port:   587,
		UseTLS: true,
		Client: stub,
	}
	prior := &Message{RFCMessageID: "orig@example.com", References: []string{"older@example.com"}}
	_, rfcMessageID, err := sender.Send(context.Background(), Mailbox{EmailAddress: "box@app.agenthook.store"}, SendRequest{
		To:          []string{"dest@example.com"},
		Subject:     "SMTP test",
		TextBody:    "hello",
		HTMLBody:    "<p>hello</p>",
		Attachments: []OutgoingAttachment{{FileName: "hello.txt", ContentType: "text/plain", ContentBase64: "aGVsbG8="}},
	}, prior)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rfcMessageID == "" {
		t.Fatalf("expected rfc message id")
	}
	if !strings.Contains(string(stub.msg), "In-Reply-To: <orig@example.com>") {
		t.Fatalf("expected reply header in SMTP raw message")
	}
	if !strings.Contains(string(stub.msg), "filename=\"hello.txt\"") {
		t.Fatalf("expected attachment in SMTP raw message")
	}
}

func TestProviderSenders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		sender        Sender
		expectedPath  string
		expectedAuth  string
		replyHeader   string
		responseBody  string
		responseCode  int
		expectErrText string
	}{
		{
			name:         "resend",
			expectedPath: "/emails",
			expectedAuth: "Bearer rk_test",
			replyHeader:  "In-Reply-To",
			responseBody: `{"id":"re_123"}`,
			responseCode: http.StatusOK,
		},
		{
			name:         "postmark",
			expectedPath: "/email",
			expectedAuth: "pm_test",
			responseBody: `{"MessageID":"pm_123"}`,
			responseCode: http.StatusOK,
		},
		{
			name:          "zeptomail failure",
			expectedPath:  "/email",
			expectedAuth:  "Zoho-enczapikey zp_test",
			replyHeader:   "In-Reply-To",
			responseBody:  `{"error":{"message":"bad request"}}`,
			responseCode:  http.StatusBadRequest,
			expectErrText: "zeptomail send failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var seenPath string
			var seenAuth string
			var seenReply string
			httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				seenPath = req.URL.Path
				seenAuth = req.Header.Get("Authorization")
				if seenAuth == "" {
					seenAuth = req.Header.Get("X-Postmark-Server-Token")
				}
				seenReply = req.Header.Get("In-Reply-To")
				return &http.Response{
					StatusCode: tc.responseCode,
					Body:       io.NopCloser(strings.NewReader(tc.responseBody)),
					Header:     make(http.Header),
				}, nil
			})}

			var sender Sender
			switch tc.name {
			case "resend":
				sender = ResendSender{APIKey: "rk_test", BaseURL: "https://api.resend.com", HTTPClient: httpClient}
			case "postmark":
				sender = PostmarkSender{ServerToken: "pm_test", BaseURL: "https://api.postmarkapp.com", HTTPClient: httpClient}
			default:
				sender = ZeptoMailSender{APIKey: "zp_test", BaseURL: "https://api.zeptomail.com/v1.1", HTTPClient: httpClient}
			}

			_, _, err := sender.Send(context.Background(), Mailbox{EmailAddress: "box@app.agenthook.store"}, SendRequest{
				To:       []string{"dest@example.com"},
				Subject:  "Provider test",
				TextBody: "hello",
				Attachments: []OutgoingAttachment{{
					FileName:      "hello.txt",
					ContentType:   "text/plain",
					ContentBase64: "aGVsbG8=",
				}},
			}, &Message{RFCMessageID: "orig@example.com"})

			if tc.expectErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.expectErrText) {
					t.Fatalf("expected error containing %q, got %v", tc.expectErrText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if seenPath != tc.expectedPath {
				t.Fatalf("expected path %q, got %q", tc.expectedPath, seenPath)
			}
			if seenAuth != tc.expectedAuth {
				t.Fatalf("expected auth %q, got %q", tc.expectedAuth, seenAuth)
			}
			if tc.replyHeader != "" && tc.name != "postmark" && seenReply == "" {
				t.Fatalf("expected reply header to be set")
			}
		})
	}
}
