package adapters

import (
	"context"
	"testing"
)

type stubUploader struct {
	last UploadRequest
}

func (s *stubUploader) Upload(_ context.Context, req UploadRequest) (UploadedAttachment, error) {
	s.last = req
	return UploadedAttachment{
		URL:         "https://cdn.example.com/" + req.Filename,
		Filename:    req.Filename,
		ContentType: req.ContentType,
		Size:        req.Size,
	}, nil
}

func TestAttachmentResolverUploadsURLOnly(t *testing.T) {
	uploader := &stubUploader{}
	resolver := NewAttachmentResolver(uploader, AttachmentPolicyError)
	input := []Attachment{
		{
			Filename:    "report.pdf",
			ContentType: "application/pdf",
			Content:     []byte("data"),
		},
	}

	resolved, err := resolver.Resolve(context.Background(), AttachmentJob{
		Channel:   "sms",
		Provider:  "twilio",
		Recipient: "+15551234567",
		EventID:   "evt-1",
	}, input)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved attachment, got %d", len(resolved))
	}
	if resolved[0].URL == "" {
		t.Fatalf("expected resolved URL to be set")
	}
	if len(resolved[0].Content) != 0 {
		t.Fatalf("expected content to be cleared after upload")
	}
	if uploader.last.Filename != "report.pdf" {
		t.Fatalf("expected upload request filename, got %s", uploader.last.Filename)
	}
}
