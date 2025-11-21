package twilio

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers SMS/WhatsApp messages via Twilio's REST API.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	client *http.Client
	cfg    Config
}

type Option func(*Adapter)

// Config captures Twilio credentials and messaging options.
type Config struct {
	AccountSID          string
	AuthToken           string
	From                string
	MessagingServiceSID string
	APIBaseURL          string
	Timeout             time.Duration
	SkipTLSVerify       bool
	PlainOnly           bool // Force text/plain when HTML is provided.
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

// WithClient allows supplying a custom HTTP client.
func WithClient(client *http.Client) Option {
	return func(a *Adapter) {
		if client != nil {
			a.client = client
		}
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "twilio",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "twilio",
			Channels: []string{"sms", "whatsapp"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			APIBaseURL: "https://api.twilio.com",
			Timeout:    10 * time.Second,
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
	if strings.TrimSpace(a.cfg.AccountSID) == "" || strings.TrimSpace(a.cfg.AuthToken) == "" {
		return fmt.Errorf("twilio: account SID/Auth token required")
	}
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return fmt.Errorf("twilio: destination missing")
	}

	from := stringValue(msg.Metadata, "from")
	if from == "" {
		from = a.cfg.From
	}

	body := stringValue(msg.Metadata, "body")
	if body == "" {
		body = msg.Body
	}
	htmlBody := stringValue(msg.Metadata, "html_body")
	if htmlBody != "" && !a.cfg.PlainOnly {
		body = stripHTML(htmlBody)
	}

	if strings.HasPrefix(strings.ToLower(msg.Channel), "whatsapp") && !strings.HasPrefix(strings.ToLower(to), "whatsapp:") {
		to = "whatsapp:" + to
		if from != "" && !strings.HasPrefix(strings.ToLower(from), "whatsapp:") {
			from = "whatsapp:" + from
		}
	}

	form := url.Values{}
	form.Set("To", to)
	if a.cfg.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", a.cfg.MessagingServiceSID)
	} else {
		if from == "" {
			return fmt.Errorf("twilio: from or messaging service SID required")
		}
		form.Set("From", from)
	}
	form.Set("Body", body)

	if media := stringSlice(msg.Metadata, "media_urls"); len(media) > 0 {
		for _, m := range media {
			form.Add("MediaUrl", m)
		}
	}

	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", strings.TrimRight(a.cfg.APIBaseURL, "/"), a.cfg.AccountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio: build request: %w", err)
	}
	req.SetBasicAuth(a.cfg.AccountSID, a.cfg.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio: request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body) // drain for connection reuse

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twilio: unexpected status %d", resp.StatusCode)
	}

	a.base.LogSuccess(a.name, msg)
	return nil
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

func stripHTML(html string) string {
	// Minimal sanitizer: drop tags to get a plain text body.
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
