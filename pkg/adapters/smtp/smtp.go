package smtp

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter is a stub SMTP messenger that logs the payload.
type Adapter struct {
	name string
	base adapters.BaseAdapter
	caps adapters.Capability
	host string
}

type Option func(*Adapter)

// WithHost configures a descriptive host label.
func WithHost(host string) Option {
	return func(a *Adapter) {
		if host != "" {
			a.host = host
		}
	}
}

// WithName overrides the provider name (defaults to smtp).
func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "smtp",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "smtp",
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
	a.base.Logger().Info(fmt.Sprintf("[smtp:%s] to=%s subject=%s", a.host, msg.To, msg.Subject))
	return nil
}
