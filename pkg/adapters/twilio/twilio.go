package twilio

import (
	"context"
	"fmt"
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
	Transport           adapters.HTTPTransportConfig
	PlainOnly           bool // Force text/plain when HTML is provided.
	DryRun              bool // When true, validates and logs but does not send.
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
		adapter.client = adapters.NewHTTPClient(adapter.cfg.Timeout, adapter.cfg.Transport)
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	accountSID, authToken := a.credentials(msg.Metadata)
	form, err := a.messageForm(msg)
	if err != nil {
		return err
	}
	if a.cfg.DryRun {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[twilio:during-dry-run] send skipped",
			"channel", msg.Channel,
		)
		return nil
	}
	if accountSID == "" || authToken == "" {
		return fmt.Errorf("twilio: account sid and auth token required")
	}

	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", strings.TrimRight(a.cfg.APIBaseURL, "/"), accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio: build request: %w", err)
	}
	req.SetBasicAuth(accountSID, authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if _, err := adapters.ExecuteRequest(a.client, "twilio", req); err != nil {
		return err
	}

	a.base.LogSuccess(a.name, msg)
	return nil
}

func (a *Adapter) credentials(metadata map[string]any) (string, string) {
	accountSID := strings.TrimSpace(firstNonEmpty(
		stringValue(metadata, "account_sid"),
		secretString(metadata, "account_sid"),
		secretString(metadata, "default"),
		a.cfg.AccountSID,
	))
	authToken := strings.TrimSpace(firstNonEmpty(
		stringValue(metadata, "auth_token"),
		secretString(metadata, "auth_token"),
		secretString(metadata, "default"),
		a.cfg.AuthToken,
	))
	return accountSID, authToken
}

func (a *Adapter) messageForm(msg adapters.Message) (url.Values, error) {
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return nil, fmt.Errorf("twilio: destination missing")
	}
	from := firstNonEmpty(stringValue(msg.Metadata, "from"), secretString(msg.Metadata, "from"), a.cfg.From)
	body := stringValue(msg.Metadata, "body")
	if body == "" {
		body = msg.Body
	}
	htmlBody := stringValue(msg.Metadata, "html_body")
	if htmlBody != "" && !a.cfg.PlainOnly {
		body = stripHTML(htmlBody)
	}
	to, from = normalizeWhatsAppAddresses(msg.Channel, to, from)

	form := url.Values{}
	form.Set("To", to)
	if err := a.setSender(form, from); err != nil {
		return nil, err
	}
	form.Set("Body", body)

	media := append(stringSlice(msg.Metadata, "media_urls"), adapters.AttachmentURLs(msg.Attachments)...)
	for _, mediaURL := range media {
		form.Add("MediaUrl", mediaURL)
	}
	if strings.TrimSpace(form.Get("Body")) == "" && len(media) == 0 {
		return nil, fmt.Errorf("twilio: body or media_urls required")
	}
	return form, nil
}

func normalizeWhatsAppAddresses(channel, to, from string) (string, string) {
	if !strings.HasPrefix(strings.ToLower(channel), "whatsapp") {
		return to, from
	}
	if !strings.HasPrefix(strings.ToLower(to), "whatsapp:") {
		to = "whatsapp:" + to
	}
	if from != "" && !strings.HasPrefix(strings.ToLower(from), "whatsapp:") {
		from = "whatsapp:" + from
	}
	return to, from
}

func (a *Adapter) setSender(form url.Values, from string) error {
	if a.cfg.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", a.cfg.MessagingServiceSID)
		return nil
	}
	if from == "" && !a.cfg.DryRun {
		return fmt.Errorf("twilio: from or messaging service SID required")
	}
	if from != "" {
		form.Set("From", from)
	}
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
