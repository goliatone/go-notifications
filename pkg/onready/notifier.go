package onready

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/notifier"
)

// OnReadyNotifier defines a DI-friendly interface for sending ready/complete events.
type OnReadyNotifier interface {
	Send(ctx context.Context, evt OnReadyEvent) error
}

// OnReadyEvent carries the payload required by ready/complete templates.
type OnReadyEvent struct {
	Recipients []string
	Locale     string
	TenantID   string
	ActorID    string
	Channels   []string

	FileName    string
	Format      string
	URL         string
	ExpiresAt   string
	Rows        int
	Parts       int
	ManifestURL string
	Message     string

	// Attachments are forwarded to adapters that support files or media URLs.
	Attachments []adapters.Attachment

	// ChannelAttachments override attachments per channel (e.g., sms media URLs).
	ChannelAttachments map[string][]adapters.Attachment

	// ChannelOverrides allow callers to pass channel-specific metadata
	// (e.g., CTA label/icon overrides) keyed by channel name.
	ChannelOverrides map[string]map[string]any
}

// notifierImpl wraps the existing notifier.Manager.
type notifierImpl struct {
	mgr            *notifier.Manager
	definitionCode string
}

// NewNotifier constructs the default OnReadyNotifier implementation.
// If definitionCode is empty, the default DefinitionCode is used.
func NewNotifier(mgr *notifier.Manager, definitionCode string) (OnReadyNotifier, error) {
	if mgr == nil {
		return nil, errors.New("onready: notifier manager is required")
	}
	code := strings.TrimSpace(definitionCode)
	if code == "" {
		code = DefinitionCode
	}
	return &notifierImpl{
		mgr:            mgr,
		definitionCode: code,
	}, nil
}

// Send enqueues and dispatches an export-ready event through the notifier manager.
func (n *notifierImpl) Send(ctx context.Context, evt OnReadyEvent) error {
	if n == nil || n.mgr == nil {
		return errors.New("onready: notifier not initialised")
	}
	if err := validateExportEvent(evt); err != nil {
		return err
	}

	payload := make(map[string]any)
	payload["file_name"] = evt.FileName
	payload["format"] = evt.Format
	payload["url"] = evt.URL
	payload["expires_at"] = evt.ExpiresAt
	if evt.Rows > 0 {
		payload["rows"] = evt.Rows
	}
	if evt.Parts > 0 {
		payload["parts"] = evt.Parts
	}
	if evt.ManifestURL != "" {
		payload["manifest_url"] = evt.ManifestURL
	}
	if evt.Message != "" {
		payload["message"] = evt.Message
	}
	if len(evt.Attachments) > 0 {
		payload["attachments"] = evt.Attachments
	}
	if len(evt.ChannelAttachments) > 0 {
		payload["channel_attachments"] = evt.ChannelAttachments
	}
	if evt.ChannelOverrides != nil {
		payload["channel_overrides"] = evt.ChannelOverrides
	}
	if evt.Locale != "" {
		payload["locale"] = evt.Locale
	}

	return n.mgr.Send(ctx, notifier.Event{
		DefinitionCode: n.definitionCode,
		Recipients:     evt.Recipients,
		Context:        payload,
		Channels:       evt.Channels,
		TenantID:       evt.TenantID,
		ActorID:        evt.ActorID,
		Locale:         evt.Locale,
		ScheduledAt:    time.Time{},
	})
}

func validateExportEvent(evt OnReadyEvent) error {
	if len(evt.Recipients) == 0 {
		return errors.New("onready: at least one recipient is required")
	}
	if strings.TrimSpace(evt.FileName) == "" {
		return errors.New("onready: FileName is required")
	}
	if strings.TrimSpace(evt.Format) == "" {
		return errors.New("onready: Format is required")
	}
	if strings.TrimSpace(evt.URL) == "" {
		return errors.New("onready: URL is required")
	}
	if strings.TrimSpace(evt.ExpiresAt) == "" {
		return errors.New("onready: ExpiresAt is required")
	}
	return nil
}
