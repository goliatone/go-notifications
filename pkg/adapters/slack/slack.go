package slack

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers messages to Slack via chat.postMessage.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
	sender Sender
}

// Config holds Slack API settings.
type Config struct {
	Token     string
	Channel   string
	BaseURL   string
	Timeout   time.Duration
	Transport adapters.HTTPTransportConfig
	DryRun    bool
}

type Option func(*Adapter)

// WithName overrides the adapter provider name.
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
			a.name = name
		}
	}
}

// WithConfig sets adapter configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithClient sets a custom HTTP client.
func WithClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the Slack adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "slack",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "slack",
			Channels: []string{"chat", "slack"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			BaseURL: "https://slack.com/api",
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.client == nil {
		adapter.client = adapters.NewHTTPClient(adapter.cfg.Timeout, adapter.cfg.Transport)
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	_, err := a.SendWithResult(ctx, msg)
	return err
}

func slackAttachments(attachments []adapters.Attachment) []map[string]any {
	files := make([]map[string]any, 0, len(attachments))
	for _, attachment := range adapters.NormalizeAttachments(attachments) {
		if attachment.URL == "" {
			continue
		}
		title := strings.TrimSpace(attachment.Filename)
		if title == "" {
			title = "attachment"
		}
		files = append(files, map[string]any{
			"title": title, "title_link": attachment.URL, "text": attachment.URL,
		})
	}
	return files
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

func stripHTML(html string) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func secretString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta["secrets"]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case map[string][]byte:
		if val, ok := v[key]; ok {
			return strings.TrimSpace(string(val))
		}
	case map[string]any:
		if val, ok := v[key]; ok {
			switch data := val.(type) {
			case string:
				return strings.TrimSpace(data)
			case []byte:
				return strings.TrimSpace(string(data))
			}
		}
	}
	return ""
}
