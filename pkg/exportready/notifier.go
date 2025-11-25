package exportready

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/notifier"
)

// ExportNotifier defines a DI-friendly interface for sending export-ready events.
type ExportNotifier interface {
	Send(ctx context.Context, evt ExportReadyEvent) error
}

// ExportReadyEvent carries the payload required by export-ready templates.
type ExportReadyEvent struct {
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

	// ChannelOverrides allow callers to pass channel-specific metadata
	// (e.g., CTA label/icon overrides) keyed by channel name.
	ChannelOverrides map[string]map[string]any
}

// notifierImpl wraps the existing notifier.Manager.
type notifierImpl struct {
	mgr            *notifier.Manager
	definitionCode string
}

// NewNotifier constructs the default ExportNotifier implementation.
// If definitionCode is empty, the default DefinitionCode is used.
func NewNotifier(mgr *notifier.Manager, definitionCode string) (ExportNotifier, error) {
	if mgr == nil {
		return nil, errors.New("exportready: notifier manager is required")
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
func (n *notifierImpl) Send(ctx context.Context, evt ExportReadyEvent) error {
	if n == nil || n.mgr == nil {
		return errors.New("exportready: notifier not initialised")
	}
	if err := validateExportEvent(evt); err != nil {
		return err
	}

	payload := make(map[string]any)
	payload["FileName"] = evt.FileName
	payload["Format"] = evt.Format
	payload["URL"] = evt.URL
	payload["ExpiresAt"] = evt.ExpiresAt
	if evt.Rows > 0 {
		payload["Rows"] = evt.Rows
	}
	if evt.Parts > 0 {
		payload["Parts"] = evt.Parts
	}
	if evt.ManifestURL != "" {
		payload["ManifestURL"] = evt.ManifestURL
	}
	if evt.Message != "" {
		payload["Message"] = evt.Message
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

func validateExportEvent(evt ExportReadyEvent) error {
	if len(evt.Recipients) == 0 {
		return errors.New("exportready: at least one recipient is required")
	}
	if strings.TrimSpace(evt.FileName) == "" {
		return errors.New("exportready: FileName is required")
	}
	if strings.TrimSpace(evt.Format) == "" {
		return errors.New("exportready: Format is required")
	}
	if strings.TrimSpace(evt.URL) == "" {
		return errors.New("exportready: URL is required")
	}
	if strings.TrimSpace(evt.ExpiresAt) == "" {
		return errors.New("exportready: ExpiresAt is required")
	}
	return nil
}
