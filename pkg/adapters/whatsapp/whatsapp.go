package whatsapp

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

// Adapter delivers WhatsApp messages via the Meta Cloud API.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

type Option func(*Adapter)

// Config captures Graph API credentials and defaults.
type Config struct {
	Token         string
	PhoneNumberID string
	APIBase       string
	Timeout       time.Duration
	Transport     adapters.HTTPTransportConfig
	PlainOnly     bool // force text/plain when HTML is provided
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
	return func(a *Adapter) { a.cfg = cfg }
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
		name: "whatsapp",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "whatsapp",
			Channels: []string{"whatsapp", "chat"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			APIBase: "https://graph.facebook.com/v19.0",
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
	if strings.TrimSpace(a.cfg.Token) == "" || strings.TrimSpace(a.cfg.PhoneNumberID) == "" {
		return fmt.Errorf("whatsapp: token and phone number id required")
	}
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return fmt.Errorf("whatsapp: destination required")
	}

	payload, err := a.messagePayload(msg, to)
	if err != nil {
		return err
	}
	bodyBytes, err := adapters.EncodeJSONPayload("whatsapp", payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/%s/messages", strings.TrimRight(a.cfg.APIBase, "/"), strings.TrimSpace(a.cfg.PhoneNumberID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.cfg.Token))

	if _, err := adapters.ExecuteRequest(a.client, "whatsapp", req); err != nil {
		return err
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func (a *Adapter) messagePayload(msg adapters.Message, to string) (map[string]any, error) {
	textBody := firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	if htmlBody != "" && !a.cfg.PlainOnly {
		textBody = stripHTML(htmlBody)
	}
	attachments := adapters.NormalizeAttachments(msg.Attachments)
	attachment := firstURLAttachment(attachments)
	if attachment == nil && textBody == "" {
		return nil, fmt.Errorf("whatsapp: body required")
	}
	if attachment != nil {
		return documentPayload(to, textBody, attachment), nil
	}
	return textPayload(to, textBody, boolValue(msg.Metadata, "preview_url")), nil
}

func documentPayload(to, caption string, attachment *adapters.Attachment) map[string]any {
	document := map[string]any{"link": attachment.URL}
	if attachment.Filename != "" {
		document["filename"] = attachment.Filename
	}
	if caption != "" {
		document["caption"] = caption
	}
	return map[string]any{
		"messaging_product": "whatsapp", "to": to,
		"type": "document", "document": document,
	}
}

func textPayload(to, body string, preview bool) map[string]any {
	text := map[string]any{"body": body}
	if preview {
		text["preview_url"] = true
	}
	return map[string]any{
		"messaging_product": "whatsapp", "to": to,
		"type": "text", "text": text,
	}
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

func boolValue(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	raw, ok := meta[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		val := strings.ToLower(strings.TrimSpace(v))
		return val == "true" || val == "1" || val == "yes"
	default:
		return false
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

func firstURLAttachment(attachments []adapters.Attachment) *adapters.Attachment {
	for i, att := range attachments {
		if strings.TrimSpace(att.URL) == "" {
			continue
		}
		return &attachments[i]
	}
	return nil
}
