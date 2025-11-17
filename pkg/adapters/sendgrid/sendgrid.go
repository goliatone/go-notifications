package sendgrid

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter logs email payloads as if they were sent via SendGrid.
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	apiKey string
}

type Option func(*Adapter)

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func WithAPIKey(key string) Option {
	return func(a *Adapter) {
		a.apiKey = key
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

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	a.base.LogSuccess(a.name, msg)
	a.base.Logger().Info(fmt.Sprintf("[sendgrid] to=%s subject=%s", msg.To, msg.Subject))
	return nil
}
