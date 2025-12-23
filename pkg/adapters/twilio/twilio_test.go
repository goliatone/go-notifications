package twilio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

func TestSendAddsMediaURLsFromAttachments(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if user != "AC123" || pass != "token" {
			t.Fatalf("unexpected basic auth %s/%s", user, pass)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.Form
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := New(&logger.Nop{}, WithConfig(Config{
		AccountSID: "AC123",
		AuthToken:  "token",
		From:       "+15551234567",
		APIBaseURL: server.URL,
	}))

	err := adapter.Send(context.Background(), adapters.Message{
		Channel: "sms",
		To:      "+15557654321",
		Body:    "hello",
		Attachments: []adapters.Attachment{
			{
				Filename: "report.pdf",
				URL:      "https://cdn.example.com/report.pdf",
			},
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotForm == nil {
		t.Fatalf("expected form values to be captured")
	}
	if gotForm.Get("To") != "+15557654321" {
		t.Fatalf("expected To to be set, got %s", gotForm.Get("To"))
	}
	media := gotForm["MediaUrl"]
	if len(media) != 1 || media[0] != "https://cdn.example.com/report.pdf" {
		t.Fatalf("expected media url to include attachment, got %v", media)
	}
}
