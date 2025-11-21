package mailgun

import (
	"context"
	"fmt"
	"io"
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
		adapter.client = &http.Client{Timeout: time.Duration(adapter.cfg.TimeoutSec) * time.Second}
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

	endpoint := fmt.Sprintf("%s/%s/messages", strings.TrimRight(a.cfg.APIBase, "/"), url.PathEscape(a.cfg.Domain))
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer mw.Close()

		_ = mw.WriteField("from", from)
		_ = mw.WriteField("to", to)
		if subj := strings.TrimSpace(msg.Subject); subj != "" {
			_ = mw.WriteField("subject", subj)
		}
		if textBody != "" {
			_ = mw.WriteField("text", textBody)
		}
		if htmlBody != "" {
			_ = mw.WriteField("html", htmlBody)
		}
		if rt := stringValue(msg.Metadata, "reply_to"); rt != "" {
			_ = mw.WriteField("h:Reply-To", rt)
		}
		if cc := stringSlice(msg.Metadata, "cc"); len(cc) > 0 {
			for _, addr := range cc {
				_ = mw.WriteField("cc", addr)
			}
		}
		if bcc := stringSlice(msg.Metadata, "bcc"); len(bcc) > 0 {
			for _, addr := range bcc {
				_ = mw.WriteField("bcc", addr)
			}
		}
		for k, v := range msg.Headers {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			_ = mw.WriteField("h:"+k, v)
		}
		// Optional raw attachments encoded as []byte in metadata:
		// metadata["attachments"] = []map[string]any{{"filename": "...", "content": []byte{...}, "content_type": "..."}}
		if atts := anySlice(msg.Metadata, "attachments"); len(atts) > 0 {
			for _, att := range atts {
				filename := stringValue(att, "filename")
				content := bytesValue(att, "content")
				if filename == "" || len(content) == 0 {
					continue
				}
				ct := stringValue(att, "content_type")
				if ct == "" {
					ct = "application/octet-stream"
				}
				header := textproto.MIMEHeader{}
				header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachment"; filename="%s"`, filename))
				header.Set("Content-Type", ct)
				part, err := mw.CreatePart(header)
				if err != nil {
					continue
				}
				_, _ = part.Write(content)
			}
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pr)
	if err != nil {
		return fmt.Errorf("mailgun: build request: %w", err)
	}
	req.SetBasicAuth("api", a.cfg.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mailgun: request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailgun: unexpected status %d", resp.StatusCode)
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

func anySlice(meta map[string]any, key string) []map[string]any {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, entry := range v {
			if m, ok := entry.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func bytesValue(meta map[string]any, key string) []byte {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []byte:
		return v
	}
	return nil
}
