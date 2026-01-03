package firebase

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

// Adapter delivers push notifications via Firebase Cloud Messaging (legacy HTTP API).
// Uses server key authentication; supports tokens, topics, or conditions.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

// Config holds FCM settings.
type Config struct {
	ServerKey string
	Endpoint  string
	Timeout   time.Duration
	DryRun    bool
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

// WithConfig sets FCM configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithClient injects a custom HTTP client.
func WithClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the Firebase adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "firebase",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "firebase",
			Channels: []string{"push", "firebase"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			Endpoint: "https://fcm.googleapis.com/fcm/send",
			Timeout:  10 * time.Second,
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
		a.base.Logger().Info("[firebase:during-dry-run] send skipped",
			"to", msg.To,
			"subject", msg.Subject,
		)
		return nil
	}
	if strings.TrimSpace(a.cfg.ServerKey) == "" {
		return fmt.Errorf("firebase: server key required")
	}

	target := strings.TrimSpace(msg.To)
	if token := stringValue(msg.Metadata, "token"); token != "" {
		target = token
	}
	topic := stringValue(msg.Metadata, "topic")
	condition := stringValue(msg.Metadata, "condition")
	if target == "" && topic == "" && condition == "" {
		return fmt.Errorf("firebase: a target is required (token, topic, or condition)")
	}

	text := firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body)
	html := firstNonEmpty(stringValue(msg.Metadata, "html_body"))

	payload := map[string]any{
		"notification": map[string]any{
			"title": msg.Subject,
			"body":  text,
		},
		"data": map[string]any{
			"text": text,
		},
		"priority": "high",
	}
	if html != "" {
		// include html in data for clients that render it
		if data, ok := payload["data"].(map[string]any); ok {
			data["html"] = html
		}
	}
	if ca := stringValue(msg.Metadata, "click_action"); ca != "" {
		if notif, ok := payload["notification"].(map[string]any); ok {
			notif["click_action"] = ca
		}
	}
	if img := stringValue(msg.Metadata, "image"); img != "" {
		if notif, ok := payload["notification"].(map[string]any); ok {
			notif["image"] = img
		}
	}
	if dataMeta, ok := msg.Metadata["data"].(map[string]any); ok {
		if data, ok := payload["data"].(map[string]any); ok {
			for k, v := range dataMeta {
				data[k] = v
			}
		}
	}

	if topic != "" {
		payload["to"] = "/topics/" + strings.TrimPrefix(topic, "/topics/")
	} else if condition != "" {
		payload["condition"] = condition
	} else {
		payload["to"] = target
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.Endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("firebase: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+strings.TrimSpace(a.cfg.ServerKey))

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("firebase: request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("firebase: unexpected status %d", resp.StatusCode)
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
