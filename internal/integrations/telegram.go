package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TelegramClient struct {
	BotToken string
	Client   *http.Client
}

func NewTelegramClient(botToken string) *TelegramClient {
	return &TelegramClient{BotToken: botToken, Client: &http.Client{Timeout: 5 * time.Second}}
}

func (t *TelegramClient) SendMessage(ctx context.Context, chatID, text string) error {
	if t.BotToken == "" {
		return fmt.Errorf("telegram bot token missing")
	}
	payload := map[string]interface{}{"chat_id": chatID, "text": text}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram status: %s", resp.Status)
	}
	return nil
}
