package smtp

import (
	"strings"
	"testing"

	"github.com/goliatone/go-notifications/pkg/adapters"
)

func TestBuildMessageWithAttachments(t *testing.T) {
	body, headers := buildMessage(
		"from@example.com",
		"to@example.com",
		"Subject",
		nil,
		nil,
		"",
		"<p>Hello</p>",
		"",
		false,
		[]adapters.Attachment{
			{
				Filename:    "report.csv",
				ContentType: "text/csv",
				Content:     []byte("a,b"),
			},
		},
	)

	if !strings.Contains(headers, "multipart/mixed") {
		t.Fatalf("expected multipart/mixed headers, got %s", headers)
	}
	if !strings.Contains(body, "Content-Type: multipart/alternative") {
		t.Fatalf("expected multipart/alternative body, got %s", body)
	}
	if !strings.Contains(body, `Content-Disposition: attachment; filename="report.csv"`) {
		t.Fatalf("expected attachment disposition, got %s", body)
	}
	if !strings.Contains(body, "Content-Transfer-Encoding: base64") {
		t.Fatalf("expected base64 encoding, got %s", body)
	}
	if !strings.Contains(body, "YSxi") {
		t.Fatalf("expected base64 content, got %s", body)
	}
}

func TestBuildMessage_HTMLOnlyDerivesText(t *testing.T) {
	body, headers := buildMessage(
		"from@example.com",
		"to@example.com",
		"Subject",
		nil,
		nil,
		"",
		"<p>Hello <strong>world</strong></p>",
		"",
		false,
		nil,
	)

	if !strings.Contains(headers, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative headers")
	}
	if !strings.Contains(body, "Content-Type: text/plain") {
		t.Fatalf("expected text/plain part")
	}
	if strings.Contains(body, "<strong>") {
		t.Fatalf("expected HTML stripped in text part")
	}
	if !strings.Contains(body, "Hello world") {
		t.Fatalf("expected derived text content")
	}
}
