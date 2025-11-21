package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter posts notifications to an HTTP endpoint (generic webhook).
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

// Config configures the webhook adapter.
type Config struct {
	URL             string
	Method          string
	Headers         map[string]string
	Timeout         time.Duration
	SkipTLSVerify   bool
	BasicAuthUser   string
	BasicAuthPass   string
	DryRun          bool
	ForwardMetadata bool // include msg.Metadata in payload
	ForwardHeaders  bool // include msg.Headers in payload
}

type Option func(*Adapter)

// WithName overrides the adapter name.
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
			a.name = name
		}
	}
}

// WithConfig sets the adapter configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithClient allows injecting a custom HTTP client.
func WithClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the webhook adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "webhook",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "webhook",
			Channels: []string{"webhook", "chat"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			Method:  "POST",
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.client == nil {
		adapter.client = &http.Client{Timeout: adapter.cfg.Timeout}
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if a.cfg.DryRun {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[webhook:during-dry-run] send skipped",
			logger.Field{Key: "url", Value: a.cfg.URL},
			logger.Field{Key: "channel", Value: msg.Channel},
		)
		return nil
	}

	if strings.TrimSpace(a.cfg.URL) == "" {
		return fmt.Errorf("webhook: url is required")
	}

	text := firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body)
	html := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	contentType := "application/json"

	payload := map[string]any{
		"channel": msg.Channel,
		"to":      msg.To,
		"subject": msg.Subject,
		"text":    text,
		"html":    html,
	}
	if a.cfg.ForwardMetadata {
		payload["metadata"] = msg.Metadata
	}
	if a.cfg.ForwardHeaders {
		payload["headers"] = msg.Headers
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(a.cfg.Method), a.cfg.URL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("webhook: build request: %w", err)
	}

	for k, v := range a.cfg.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	if a.cfg.BasicAuthUser != "" {
		req.SetBasicAuth(a.cfg.BasicAuthUser, a.cfg.BasicAuthPass)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}

	a.base.LogSuccess(a.name, msg)
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
