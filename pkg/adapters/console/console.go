package console

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter writes notifications to the configured logger/stdout for debugging.
type Adapter struct {
	name string
	base adapters.BaseAdapter
	caps adapters.Capability
	opts Options
}

type Option func(*Adapter)

// Options tweak console output.
type Options struct {
	Structured bool // when true, emit a structured log map instead of a formatted string
}

// WithName overrides the adapter provider name (defaults to "console").
func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

// WithStructured enables structured logging mode.
func WithStructured(enabled bool) Option {
	return func(a *Adapter) {
		a.opts.Structured = enabled
	}
}

// New constructs a console adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "console",
		caps: adapters.Capability{
			Name:     "console",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
	}
	adapter.base = adapters.NewBaseAdapter(l)
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// Name implements adapters.Messenger.
func (a *Adapter) Name() string {
	return a.name
}

// Capabilities implements adapters.Messenger.
func (a *Adapter) Capabilities() adapters.Capability {
	return a.caps
}

// Send logs the rendered message to the configured logger.
func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	textBody := firstNonEmpty(msg.Body, stringValue(msg.Metadata, "text_body"), stringValue(msg.Metadata, "body"))
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	contentType := stringValue(msg.Metadata, "content_type")

	if a.opts.Structured {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("console delivery",
			logger.Field{Key: "channel", Value: msg.Channel},
			logger.Field{Key: "to", Value: msg.To},
			logger.Field{Key: "subject", Value: msg.Subject},
			logger.Field{Key: "text", Value: textBody},
			logger.Field{Key: "html", Value: htmlBody},
			logger.Field{Key: "content_type", Value: contentType},
			logger.Field{Key: "metadata", Value: msg.Metadata},
		)
		return nil
	}

	format := "plain"
	bodyToPrint := textBody
	if htmlBody != "" && strings.Contains(strings.ToLower(contentType), "html") {
		format = "html"
		bodyToPrint = htmlBody
	}

	fmtStr := "[console][%s][%s] subject=%s to=%s body=%s"
	a.base.LogSuccess(a.name, msg)
	a.base.Logger().Info(fmt.Sprintf(fmtStr, msg.Channel, format, msg.Subject, msg.To, bodyToPrint))
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func stringValue(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
