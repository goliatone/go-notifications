package smtp

import (
	"net/mail"
	"strings"
	"testing"

	"github.com/goliatone/go-notifications/pkg/adapters"
)

func mustParseAddress(t *testing.T, value string) *mail.Address {
	t.Helper()
	addr, err := mail.ParseAddress(value)
	if err != nil {
		t.Fatalf("parse address %q: %v", value, err)
	}
	return addr
}

func TestComposeMessageWithAttachments(t *testing.T) {
	message, err := composeMessage(composeMessageInput{
		From:      mustParseAddress(t, "from@example.com"),
		To:        mustParseAddress(t, "to@example.com"),
		Subject:   "Subject",
		TextBody:  "",
		HTMLBody:  "<p>Hello</p>",
		PlainOnly: false,
		Attachments: []adapters.Attachment{
			{
				Filename:    "report.csv",
				ContentType: "text/csv",
				Content:     []byte("a,b"),
			},
		},
	})
	if err != nil {
		t.Fatalf("compose message: %v", err)
	}
	payload := string(message)
	if !strings.Contains(payload, "multipart/mixed") {
		t.Fatalf("expected multipart/mixed payload, got %s", payload)
	}
	if !strings.Contains(payload, "Content-Type: multipart/alternative") {
		t.Fatalf("expected multipart/alternative part, got %s", payload)
	}
	if !strings.Contains(payload, `Content-Disposition: attachment; filename="report.csv"`) {
		t.Fatalf("expected attachment disposition, got %s", payload)
	}
	if !strings.Contains(payload, "Content-Transfer-Encoding: base64") {
		t.Fatalf("expected base64 attachment encoding, got %s", payload)
	}
	if !strings.Contains(payload, "YSxi") {
		t.Fatalf("expected base64 content, got %s", payload)
	}
}

func TestComposeMessageHTMLOnlyDerivesText(t *testing.T) {
	message, err := composeMessage(composeMessageInput{
		From:      mustParseAddress(t, "from@example.com"),
		To:        mustParseAddress(t, "to@example.com"),
		Subject:   "Subject",
		HTMLBody:  "<p>Hello <strong>world</strong></p>",
		PlainOnly: false,
	})
	if err != nil {
		t.Fatalf("compose message: %v", err)
	}
	body := string(message)
	if !strings.Contains(body, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative headers")
	}
	if !strings.Contains(body, "Content-Type: text/plain") {
		t.Fatalf("expected text/plain part")
	}
	plainHeader := "Content-Type: text/plain; charset=UTF-8"
	htmlHeader := "Content-Type: text/html; charset=UTF-8"
	plainIdx := strings.Index(body, plainHeader)
	htmlIdx := strings.Index(body, htmlHeader)
	if plainIdx == -1 || htmlIdx == -1 || htmlIdx < plainIdx {
		t.Fatalf("expected text/plain part before text/html part")
	}
	plainSection := body[plainIdx:htmlIdx]
	if strings.Contains(plainSection, "<strong>") {
		t.Fatalf("expected HTML stripped in text part")
	}
	plainLower := strings.ToLower(plainSection)
	if !strings.Contains(plainLower, "hello") || !strings.Contains(plainLower, "world") {
		t.Fatalf("expected derived text content")
	}
}

func TestComposeMessageRejectsCRLFInSubject(t *testing.T) {
	_, err := composeMessage(composeMessageInput{
		From:     mustParseAddress(t, "from@example.com"),
		To:       mustParseAddress(t, "to@example.com"),
		Subject:  "ok\r\nInjected: bad",
		TextBody: "hello",
	})
	if err == nil {
		t.Fatalf("expected CRLF subject rejection")
	}
}

func TestComposeMessageRejectsUnsafeAttachmentFilename(t *testing.T) {
	_, err := composeMessage(composeMessageInput{
		From:     mustParseAddress(t, "from@example.com"),
		To:       mustParseAddress(t, "to@example.com"),
		Subject:  "Subject",
		TextBody: "hello",
		Attachments: []adapters.Attachment{
			{Filename: "evil\nfile.txt", Content: []byte("x")},
		},
	})
	if err == nil {
		t.Fatalf("expected unsafe filename rejection")
	}
}

func TestParseAddressListRejectsCRLF(t *testing.T) {
	_, err := parseAddressList([]string{"ok@example.com\r\nBcc:evil@example.com"})
	if err == nil {
		t.Fatalf("expected CRLF address rejection")
	}
}
