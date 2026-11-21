package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"xops-aegis-guard/config"
)

type notificationTransport func(*http.Request) (*http.Response, error)

func (transport notificationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func TestNotifyRobotDeliveryResult(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "empty success", status: 204},
		{name: "code zero", status: 200, body: `{"code":0}`},
		{name: "code 200", status: 200, body: `{"code":200}`},
		{name: "relay rejection with HTTP success", status: 200, body: `{"code":-1}`, wantErr: "code -1"},
		{name: "HTTP failure", status: 502, wantErr: "status 502"},
	} {
		t.Run(test.name, func(t *testing.T) {
			notifier := NewNotifier(config.Robot{Enabled: true, URL: "http://robot.local", SourceName: "ANQS"})
			notifier.client.Transport = notificationTransport(func(req *http.Request) (*http.Response, error) {
				var payload struct {
					RobotID int    `json:"im_id"`
					Text    string `json:"text"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if req.Method != http.MethodPost || req.URL.Path != "/v1/message" || payload.RobotID != 42 || payload.Text != "告警确认" {
					t.Fatalf("unexpected delivery: %s %s %+v", req.Method, req.URL.Path, payload)
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})
			err := notifier.Notify(context.Background(), 42, "告警确认")
			if test.wantErr == "" && err != nil || test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestNotifyTelegramDeliveryResult(t *testing.T) {
	const token = "12345:example-token"
	t.Run("long confirmation accepted", func(t *testing.T) {
		message := strings.Repeat("确认告警", 600)
		notifier := NewNotifier(config.Robot{}, config.Telegram{Enabled: true, BotToken: token, ChatID: "-100123"})
		notifier.client.Transport = notificationTransport(func(req *http.Request) (*http.Response, error) {
			var payload struct {
				ChatID string `json:"chat_id"`
				Text   string `json:"text"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if req.URL.Host != "api.telegram.org" || payload.ChatID != "-100123" || payload.Text != message {
				t.Fatal("unexpected Telegram delivery request")
			}
			body := `{"ok":true,"result":{"text":"` + payload.Text + `"}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if err := notifier.NotifyTelegram(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rejection exposes reason and redacts token", func(t *testing.T) {
		notifier := NewNotifier(config.Robot{}, config.Telegram{Enabled: true, BotToken: token, ChatID: "-100123"})
		notifier.client.Transport = notificationTransport(func(req *http.Request) (*http.Response, error) {
			body := `{"ok":false,"description":"Forbidden: bot cannot send messages ` + token + `"}`
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		err := notifier.NotifyTelegram(context.Background(), "告警确认")
		if err == nil || !strings.Contains(err.Error(), "bot cannot send messages") || strings.Contains(err.Error(), token) {
			t.Fatalf("unexpected Telegram rejection: %v", err)
		}
	})
}
