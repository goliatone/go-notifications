package sendgrid

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

// Adapter delivers email via the SendGrid API.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

type Option func(*Adapter)

// Config holds SendGrid credentials and defaults.
type Config struct {
	APIKey     string
	BaseURL    string
	From       string
	ReplyTo    string
	TimeoutSec int
	Transport  adapters.HTTPTransportConfig
}

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func WithAPIKey(key string) Option {
	return func(a *Adapter) {
		a.cfg.APIKey = key
	}
}

func WithFrom(from string) Option {
	return func(a *Adapter) {
		a.cfg.From = from
	}
}

func WithReplyTo(replyTo string) Option {
	return func(a *Adapter) {
		a.cfg.ReplyTo = replyTo
	}
}

func WithBaseURL(url string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(url) != "" {
			a.cfg.BaseURL = strings.TrimRight(url, "/")
		}
	}
}

func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

func WithTimeout(seconds int) Option {
	return func(a *Adapter) {
		if seconds > 0 {
			a.cfg.TimeoutSec = seconds
		}
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "sendgrid",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "sendgrid",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			BaseURL:    "https://api.sendgrid.com/v3",
			TimeoutSec: 10,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	if adapter.client == nil {
		adapter.client = adapters.NewHTTPClient(time.Duration(adapter.cfg.TimeoutSec)*time.Second, adapter.cfg.Transport)
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	apiKey := strings.TrimSpace(firstNonEmpty(
		stringValue(msg.Metadata, "api_key"),
		secretString(msg.Metadata, "api_key"),
		secretString(msg.Metadata, "default"),
		a.cfg.APIKey,
	))
	if apiKey == "" {
		return fmt.Errorf("sendgrid: api key required")
	}
	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("sendgrid: destination required")
	}
	from := firstNonEmpty(
		stringValue(msg.Metadata, "from"),
		secretString(msg.Metadata, "from"),
		a.cfg.From,
	)
	if strings.TrimSpace(from) == "" {
		return fmt.Errorf("sendgrid: from required")
	}

	textBody := firstNonEmpty(stringValue(msg.Metadata, "text_body"), stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))

	personalization := map[string]any{
		"to": []map[string]string{{"email": msg.To}},
	}
	if rt := firstNonEmpty(stringValue(msg.Metadata, "reply_to"), a.cfg.ReplyTo); rt != "" {
		personalization["reply_to"] = map[string]string{"email": rt}
	}

	content := []map[string]string{}
	if textBody != "" {
		content = append(content, map[string]string{"type": "text/plain", "value": textBody})
	}
	if htmlBody != "" {
		content = append(content, map[string]string{"type": "text/html", "value": htmlBody})
	}
	if len(content) == 0 {
		return fmt.Errorf("sendgrid: content empty")
	}

	requestBody := map[string]any{
		"personalizations": []any{personalization},
		"from":             map[string]string{"email": from},
		"subject":          msg.Subject,
		"content":          content,
	}

	if hdrs := msg.Headers; len(hdrs) > 0 {
		requestBody["headers"] = hdrs
	}

	addRecipients(personalization, "cc", stringSlice(msg.Metadata, "cc"))
	addRecipients(personalization, "bcc", stringSlice(msg.Metadata, "bcc"))

	bodyBytes, err := adapters.EncodeJSONPayload("sendgrid", requestBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+"/mail/send", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("sendgrid: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	if _, err := adapters.ExecuteRequest(a.client, "sendgrid", req); err != nil {
		return err
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func addRecipients(personalization map[string]any, field string, addresses []string) {
	if len(addresses) == 0 {
		return
	}
	recipients := make([]map[string]string, 0, len(addresses))
	for _, address := range addresses {
		recipients = append(recipients, map[string]string{"email": address})
	}
	personalization[field] = recipients
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

func stringSlice(meta map[string]any, key string) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			out = append(out, strings.TrimSpace(fmt.Sprint(entry)))
		}
		return out
	default:
		return nil
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
