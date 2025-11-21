package telegram

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers messages through the Telegram Bot API.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	client *http.Client
	cfg    Config
}

type Option func(*Adapter)

// Config holds Telegram Bot API options.
type Config struct {
	Token                 string
	BaseURL               string
	ParseMode             string
	DisableWebPagePreview bool
	DisableNotification   bool
	Timeout               time.Duration
	SkipTLSVerify         bool
	PlainOnly             bool // force text/plain even when HTML is provided
}

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
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

// WithClient allows injecting a custom HTTP client.
func WithClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "telegram",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "telegram",
			Channels: []string{"chat"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			BaseURL: "https://api.telegram.org",
			Timeout: 10 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.client == nil {
		adapter.client = &http.Client{
			Timeout: adapter.cfg.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: adapter.cfg.SkipTLSVerify},
			},
		}
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if strings.TrimSpace(a.cfg.Token) == "" {
		return fmt.Errorf("telegram: bot token required")
	}
	chatID := strings.TrimSpace(msg.To)
	if chatID == "" {
		return fmt.Errorf("telegram: chat id required")
	}

	body := firstNonEmpty(msg.Metadata, "html_body", "body", msg.Body)
	parseMode := strings.TrimSpace(firstNonEmpty(msg.Metadata, "parse_mode", a.cfg.ParseMode))
	if parseMode == "" && !a.cfg.PlainOnly && strings.TrimSpace(firstNonEmpty(msg.Metadata, "html_body")) != "" {
		parseMode = "HTML"
	}
	if a.cfg.PlainOnly {
		parseMode = ""
		body = firstNonEmpty(msg.Metadata, "body", msg.Body)
	}
	if body == "" {
		return fmt.Errorf("telegram: message body required")
	}

	payload := map[string]any{
		"chat_id": chatID,
		"text":    body,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}

	disablePreview := boolValue(msg.Metadata, "disable_preview", a.cfg.DisableWebPagePreview)
	if disablePreview {
		payload["disable_web_page_preview"] = true
	}
	disableNotification := boolValue(msg.Metadata, "silent", a.cfg.DisableNotification)
	if disableNotification {
		payload["disable_notification"] = true
	}
	if thread := stringValue(msg.Metadata, "thread_id"); thread != "" {
		payload["message_thread_id"] = thread
	}
	if replyTo := stringValue(msg.Metadata, "reply_to"); replyTo != "" {
		payload["reply_to_message_id"] = replyTo
	}

	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(a.cfg.BaseURL, "/"), strings.TrimSpace(a.cfg.Token))
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body) // drain for keep-alive reuse

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func firstNonEmpty(meta map[string]any, keys ...any) string {
	for _, key := range keys {
		switch v := key.(type) {
		case string:
			if s := stringValue(meta, v); s != "" {
				return s
			}
		default:
			if s := fmt.Sprint(v); strings.TrimSpace(s) != "" {
				return s
			}
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

func boolValue(meta map[string]any, key string, def bool) bool {
	if meta == nil {
		return def
	}
	raw, ok := meta[key]
	if !ok {
		return def
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		val := strings.ToLower(strings.TrimSpace(v))
		return val == "true" || val == "1" || val == "yes"
	default:
		return def
	}
}
