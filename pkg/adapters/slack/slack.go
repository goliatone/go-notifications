package slack

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

// Adapter delivers messages to Slack via chat.postMessage.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client *http.Client
}

// Config holds Slack API settings.
type Config struct {
	Token         string
	Channel       string
	BaseURL       string
	Timeout       time.Duration
	SkipTLSVerify bool
	DryRun        bool
}

type Option func(*Adapter)

// WithName overrides the adapter provider name.
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
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

// WithClient sets a custom HTTP client.
func WithClient(c *http.Client) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the Slack adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "slack",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "slack",
			Channels: []string{"chat", "slack"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			BaseURL: "https://slack.com/api",
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
	token := strings.TrimSpace(firstNonEmpty(
		stringValue(msg.Metadata, "token"),
		secretString(msg.Metadata, "token"),
		secretString(msg.Metadata, "default"),
		a.cfg.Token,
	))
	if token == "" && !a.cfg.DryRun {
		return fmt.Errorf("slack: token required")
	}
	channel := firstNonEmpty(stringValue(msg.Metadata, "channel"), a.cfg.Channel)
	if channel == "" {
		return fmt.Errorf("slack: channel required")
	}

	text := firstNonEmpty(stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	if htmlBody != "" && !a.cfg.DryRun {
		// Slack uses mrkdwn; strip tags to keep content readable
		text = stripHTML(htmlBody)
	}
	if text == "" {
		return fmt.Errorf("slack: message body required")
	}

	payload := map[string]any{
		"channel": channel,
		"text":    text,
		"mrkdwn":  true,
	}
	if thread := stringValue(msg.Metadata, "thread_ts"); thread != "" {
		payload["thread_ts"] = thread
	}
	if attachments := adapters.NormalizeAttachments(msg.Attachments); len(attachments) > 0 {
		files := make([]map[string]any, 0, len(attachments))
		for _, att := range attachments {
			if att.URL == "" {
				continue
			}
			title := strings.TrimSpace(att.Filename)
			if title == "" {
				title = "attachment"
			}
			files = append(files, map[string]any{
				"title":      title,
				"title_link": att.URL,
				"text":       att.URL,
			})
		}
		if len(files) > 0 {
			payload["attachments"] = files
		}
	}

	if a.cfg.DryRun || token == "" {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[slack:during-dry-run] send skipped",
			"channel", channel,
			"text", text,
		)
		return nil
	}

	bodyBytes, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(a.cfg.BaseURL, "/") + "/chat.postMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack: unexpected status %d", resp.StatusCode)
	}

	// Slack returns ok=false on logical errors
	var apiResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(data, &apiResp)
	if !apiResp.OK {
		return fmt.Errorf("slack: api error: %s", apiResp.Error)
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
