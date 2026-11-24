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
		{name: "malformed response", status: 200, body: `{"code":`, wantErr: "decode"},
		{name: "proxy HTML", status: 200, body: `<html>login</html>`, wantErr: "decode"},
		{name: "oversized response", status: 200, body: strings.Repeat(" ", 65537), wantErr: "size limit"},
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

func TestNotifyDisabled(t *testing.T) {
	for _, notifier := range []*Notifier{nil, NewNotifier(config.Robot{})} {
		if err := notifier.Notify(context.Background(), 42, "告警确认"); err == nil {
			t.Fatal("disabled notifier must not report successful delivery")
		}
	}
}
