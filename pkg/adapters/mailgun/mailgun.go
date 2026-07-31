package mailgun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers email via the Mailgun API.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

// Config holds Mailgun credentials and defaults.
type Config struct {
	Domain     string
	APIKey     string
	APIBase    string
	From       string
	TimeoutSec int
	Transport  adapters.HTTPTransportConfig
}

type Option func(*Adapter)

// WithName overrides the provider name (defaults to "mailgun").
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
			a.name = name
		}
	}
}

// WithConfig sets the adapter configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) { a.cfg = cfg }
}

// WithHTTPClient injects a custom client.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs a Mailgun adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "mailgun",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "mailgun",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			APIBase:    "https://api.mailgun.net/v3",
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

// Name implements adapters.Messenger.
func (a *Adapter) Name() string { return a.name }

// Capabilities implements adapters.Messenger.
func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

// Send posts the message to Mailgun's Messages endpoint.
func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if strings.TrimSpace(a.cfg.Domain) == "" || strings.TrimSpace(a.cfg.APIKey) == "" {
		return fmt.Errorf("mailgun: domain and api key required")
	}
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return fmt.Errorf("mailgun: destination required")
	}

	from := firstNonEmpty(stringValue(msg.Metadata, "from"), a.cfg.From)
	if strings.TrimSpace(from) == "" {
		return fmt.Errorf("mailgun: from required")
	}

	textBody := firstNonEmpty(stringValue(msg.Metadata, "text_body"), stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	if textBody == "" && htmlBody == "" {
		return fmt.Errorf("mailgun: content empty")
	}

	attachments := adapters.EmailAttachments(msg.Attachments)
	if metaAttachments := adapters.AttachmentsFromValue(msg.Metadata["attachments"]); len(metaAttachments) > 0 {
		attachments = append(attachments, metaAttachments...)
		attachments = adapters.EmailAttachments(attachments)
	}

	endpoint := fmt.Sprintf("%s/%s/messages", strings.TrimRight(a.cfg.APIBase, "/"), url.PathEscape(a.cfg.Domain))
	body, contentType, err := buildMultipartBody(msg, from, to, textBody, htmlBody, attachments)
	if err != nil {
		return fmt.Errorf("mailgun: build multipart body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("mailgun: build request: %w", err)
	}
	req.SetBasicAuth("api", a.cfg.APIKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mailgun: request failed: %w", err)
	}
	respBody, err := adapters.ReadResponseBody(resp)
	closeErr := resp.Body.Close()
	if responseErr := errors.Join(err, closeErr); responseErr != nil {
		return fmt.Errorf("mailgun: %w", responseErr)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return adapters.HTTPStatusError("mailgun", resp.StatusCode, respBody)
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func buildMultipartBody(
	msg adapters.Message,
	from string,
	to string,
	textBody string,
	htmlBody string,
	attachments []adapters.Attachment,
) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := [][2]string{
		{"from", from},
		{"to", to},
		{"subject", strings.TrimSpace(msg.Subject)},
		{"text", textBody},
		{"html", htmlBody},
		{"h:Reply-To", stringValue(msg.Metadata, "reply_to")},
	}
	for _, field := range fields {
		if field[1] == "" {
			continue
		}
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", err
		}
	}
	for _, address := range stringSlice(msg.Metadata, "cc") {
		if err := writer.WriteField("cc", address); err != nil {
			return nil, "", err
		}
	}
	for _, address := range stringSlice(msg.Metadata, "bcc") {
		if err := writer.WriteField("bcc", address); err != nil {
			return nil, "", err
		}
	}
	for key, value := range msg.Headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		if err := writer.WriteField("h:"+key, value); err != nil {
			return nil, "", err
		}
	}
	if err := writeAttachments(writer, attachments); err != nil {
		return nil, "", err
	}
	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, contentType, nil
}

func writeAttachments(writer *multipart.Writer, attachments []adapters.Attachment) error {
	for _, attachment := range attachments {
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(
			`form-data; name="attachment"; filename="%s"`,
			attachment.Filename,
		))
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err := part.Write(attachment.Content); err != nil {
			return err
		}
	}
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
