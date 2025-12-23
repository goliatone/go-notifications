package adapters

import (
	"context"
	"fmt"
	"strings"
)

// AttachmentJob provides context for resolving attachments.
type AttachmentJob struct {
	Channel        string
	Provider       string
	Recipient      string
	EventID        string
	DefinitionCode string
}

// UploadRequest captures the data needed to upload raw attachment content.
type UploadRequest struct {
	Filename    string
	ContentType string
	Content     []byte
	Size        int
	Channel     string
	Provider    string
	Recipient   string
	EventID     string
	Metadata    map[string]any
}

// UploadedAttachment is the result of a successful upload.
type UploadedAttachment struct {
	URL         string
	Filename    string
	ContentType string
	Size        int
}

// AttachmentUploader uploads raw attachment data and returns a URL.
type AttachmentUploader interface {
	Upload(ctx context.Context, req UploadRequest) (UploadedAttachment, error)
}

// AttachmentResolver converts attachments into channel-ready forms (e.g., URL-only).
type AttachmentResolver interface {
	Resolve(ctx context.Context, job AttachmentJob, attachments []Attachment) ([]Attachment, error)
}

// AttachmentPolicy controls how unsupported attachments are handled.
type AttachmentPolicy string

const (
	AttachmentPolicyDrop  AttachmentPolicy = "drop"
	AttachmentPolicyError AttachmentPolicy = "error"
)

// Resolver resolves attachments using an uploader when URLs are required.
type Resolver struct {
	Uploader AttachmentUploader
	Policy   AttachmentPolicy
}

// NewAttachmentResolver constructs a resolver with optional uploader/policy.
func NewAttachmentResolver(uploader AttachmentUploader, policy AttachmentPolicy) *Resolver {
	if policy == "" {
		policy = AttachmentPolicyDrop
	}
	return &Resolver{
		Uploader: uploader,
		Policy:   policy,
	}
}

// Resolve normalizes attachments and uploads raw content for URL-only channels.
func (r *Resolver) Resolve(ctx context.Context, job AttachmentJob, attachments []Attachment) ([]Attachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	normalized := NormalizeAttachments(attachments)
	if len(normalized) == 0 {
		return nil, nil
	}
	if !requiresURL(job.Channel) {
		return normalized, nil
	}
	out := make([]Attachment, 0, len(normalized))
	for _, att := range normalized {
		if strings.TrimSpace(att.URL) != "" {
			att.Content = nil
			out = append(out, att)
			continue
		}
		if len(att.Content) == 0 {
			continue
		}
		if r == nil || r.Uploader == nil {
			if r != nil && r.Policy == AttachmentPolicyError {
				return nil, fmt.Errorf("attachments: missing uploader for channel %s", job.Channel)
			}
			continue
		}
		uploaded, err := r.Uploader.Upload(ctx, UploadRequest{
			Filename:    att.Filename,
			ContentType: att.ContentType,
			Content:     att.Content,
			Size:        att.Size,
			Channel:     job.Channel,
			Provider:    job.Provider,
			Recipient:   job.Recipient,
			EventID:     job.EventID,
		})
		if err != nil {
			return nil, err
		}
		att.URL = strings.TrimSpace(uploaded.URL)
		att.Content = nil
		if uploaded.Filename != "" {
			att.Filename = uploaded.Filename
		}
		if uploaded.ContentType != "" {
			att.ContentType = uploaded.ContentType
		}
		if uploaded.Size > 0 {
			att.Size = uploaded.Size
		}
		if att.URL == "" {
			if r.Policy == AttachmentPolicyError {
				return nil, fmt.Errorf("attachments: upload returned empty url for channel %s", job.Channel)
			}
			continue
		}
		out = append(out, att)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func requiresURL(channel string) bool {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "sms", "mms", "whatsapp", "chat", "slack", "telegram":
		return true
	default:
		return false
	}
}
