package aws_ses

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter delivers email via AWS SES.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	cfg    Config
	client SESClient
}

// Config holds SES settings.
type Config struct {
	From             string
	Region           string
	Profile          string
	ConfigurationSet string
	DryRun           bool
}

type Option func(*Adapter)

// SESClient abstracts the SES client for testing.
type SESClient interface {
	SendEmail(ctx context.Context, params *ses.SendEmailInput, optFns ...func(*ses.Options)) (*ses.SendEmailOutput, error)
}

// WithName overrides the adapter provider name.
func WithName(name string) Option {
	return func(a *Adapter) {
		if strings.TrimSpace(name) != "" {
			a.name = name
		}
	}
}

// WithConfig sets the adapter configuration.
func WithConfig(cfg Config) Option {
	return func(a *Adapter) {
		a.cfg = cfg
	}
}

// WithClient injects a custom SES client.
func WithClient(c SESClient) Option {
	return func(a *Adapter) {
		if c != nil {
			a.client = c
		}
	}
}

// New constructs the SES adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "aws_ses",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "aws_ses",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
		cfg: Config{
			Region: "us-east-1",
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) ensureClient(ctx context.Context) error {
	if a.client != nil {
		return nil
	}
	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(a.cfg.Region),
	}
	if a.cfg.Profile != "" {
		loadOpts = append(loadOpts, config.WithSharedConfigProfile(a.cfg.Profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return fmt.Errorf("aws_ses: load config: %w", err)
	}
	a.client = ses.NewFromConfig(cfg, func(o *ses.Options) {
		o.RetryMaxAttempts = 3
	})
	return nil
}

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	if a.cfg.DryRun {
		a.base.LogSuccess(a.name, msg)
		a.base.Logger().Info("[aws_ses:during-dry-run] send skipped",
			"to", msg.To,
			"subject", msg.Subject,
		)
		return nil
	}

	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("aws_ses: destination required")
	}
	from := firstNonEmpty(stringValue(msg.Metadata, "from"), a.cfg.From)
	if strings.TrimSpace(from) == "" {
		return fmt.Errorf("aws_ses: from required")
	}
	textBody := firstNonEmpty(stringValue(msg.Metadata, "text_body"), stringValue(msg.Metadata, "body"), msg.Body)
	htmlBody := firstNonEmpty(stringValue(msg.Metadata, "html_body"))
	if textBody == "" && htmlBody == "" {
		return fmt.Errorf("aws_ses: content empty")
	}

	if err := a.ensureClient(ctx); err != nil {
		return err
	}

	input := &ses.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses:  []string{strings.TrimSpace(msg.To)},
			CcAddresses:  stringSlice(msg.Metadata, "cc"),
			BccAddresses: stringSlice(msg.Metadata, "bcc"),
		},
		Source: aws.String(from),
		Message: &types.Message{
			Subject: &types.Content{Data: aws.String(msg.Subject)},
			Body: &types.Body{
				Text: textContent(textBody),
				Html: htmlContent(htmlBody),
			},
		},
	}
	if cs := strings.TrimSpace(a.cfg.ConfigurationSet); cs != "" {
		input.ConfigurationSetName = aws.String(cs)
	}

	_, err := a.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("aws_ses: send email: %w", err)
	}
	a.base.LogSuccess(a.name, msg)
	return nil
}

func textContent(body string) *types.Content {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return &types.Content{Data: aws.String(body)}
}

func htmlContent(body string) *types.Content {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return &types.Content{Data: aws.String(body)}
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
