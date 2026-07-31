package telegram

import (
	"bytes"
	"context"
	"fmt"
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
	ChatID                string
	BaseURL               string
	ParseMode             string
	DisableWebPagePreview bool
	DisableNotification   bool
	Timeout               time.Duration
	Transport             adapters.HTTPTransportConfig
	PlainOnly             bool // force text/plain even when HTML is provided
	DryRun                bool // when true, skip sending but still succeed
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
		adapter.client = adapters.NewHTTPClient(adapter.cfg.Timeout, adapter.cfg.Transport)
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	content, err := a.prepareContent(msg)
	if err != nil {
		return err
	}
	if a.cfg.DryRun {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[telegram:during-dry-run] send skipped",
			"channel", msg.Channel,
		)
		return nil
	}
	if content.token == "" {
		return fmt.Errorf("telegram: bot token required")
	}

	payload, endpoint := a.telegramRequest(content, msg.Metadata)
	bodyBytes, err := adapters.EncodeJSONPayload("telegram", payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if _, err := adapters.ExecuteRequest(a.client, "telegram", req); err != nil {
		return err
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

type telegramContent struct {
	token      string
	chatID     string
	body       string
	parseMode  string
	attachment *adapters.Attachment
}

func (a *Adapter) prepareContent(msg adapters.Message) (telegramContent, error) {
	content := telegramContent{}
	content.token = strings.TrimSpace(firstNonEmptyStrings(
		stringValue(msg.Metadata, "token"),
		secretString(msg.Metadata, "token"),
		secretString(msg.Metadata, "default"),
		a.cfg.Token,
	))
	content.chatID = strings.TrimSpace(firstNonEmptyStrings(stringValue(msg.Metadata, "chat_id"), msg.To, a.cfg.ChatID))
	if content.chatID == "" {
		return telegramContent{}, fmt.Errorf("telegram: chat id required")
	}
	htmlBody := stringValue(msg.Metadata, "html_body")
	textBody := stringValue(msg.Metadata, "body")
	content.body = firstNonEmptyStrings(htmlBody, textBody, msg.Body, msg.Subject)
	content.parseMode = sanitizeParseMode(strings.TrimSpace(firstNonEmptyStrings(stringValue(msg.Metadata, "parse_mode"), a.cfg.ParseMode)))
	if content.parseMode == "" && !a.cfg.PlainOnly && strings.TrimSpace(htmlBody) != "" {
		content.parseMode = "HTML"
	}
	if a.cfg.PlainOnly {
		content.parseMode = ""
		content.body = firstNonEmptyStrings(textBody, msg.Body, msg.Subject)
	}
	content.attachment = firstURLAttachment(adapters.NormalizeAttachments(msg.Attachments))
	if content.attachment == nil && content.body == "" {
		return telegramContent{}, fmt.Errorf("telegram: message body required")
	}
	return content, nil
}

func (a *Adapter) telegramRequest(content telegramContent, metadata map[string]any) (map[string]any, string) {
	payload := map[string]any{"chat_id": content.chatID}
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(a.cfg.BaseURL, "/"), content.token)
	if content.attachment != nil {
		applyTelegramDocument(payload, content)
		endpoint = fmt.Sprintf("%s/bot%s/sendDocument", strings.TrimRight(a.cfg.BaseURL, "/"), content.token)
	} else {
		applyTelegramText(payload, content, boolValue(metadata, "disable_preview", a.cfg.DisableWebPagePreview))
	}
	disableNotification := boolValue(metadata, "silent", a.cfg.DisableNotification)
	if disableNotification {
		payload["disable_notification"] = true
	}
	if thread := stringValue(metadata, "thread_id"); thread != "" {
		payload["message_thread_id"] = thread
	}
	if replyTo := stringValue(metadata, "reply_to"); replyTo != "" {
		payload["reply_to_message_id"] = replyTo
	}
	return payload, endpoint
}

func applyTelegramDocument(payload map[string]any, content telegramContent) {
	payload["document"] = content.attachment.URL
	if content.body != "" {
		payload["caption"] = content.body
	}
	if content.parseMode != "" {
		payload["parse_mode"] = content.parseMode
	}
}

func applyTelegramText(payload map[string]any, content telegramContent, disablePreview bool) {
	payload["text"] = content.body
	if content.parseMode != "" {
		payload["parse_mode"] = content.parseMode
	}
	if disablePreview {
		payload["disable_web_page_preview"] = true
	}
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

func sanitizeParseMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "html":
		return "HTML"
	case "markdown", "md", "markdownv2", "mdv2":
		return "MarkdownV2"
	default:
		return ""
	}
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

func firstNonEmptyStrings(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstURLAttachment(attachments []adapters.Attachment) *adapters.Attachment {
	for i, att := range attachments {
		if strings.TrimSpace(att.URL) == "" {
			continue
		}
		return &attachments[i]
	}
	return nil
}
