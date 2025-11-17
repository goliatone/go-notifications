package console

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter writes notifications to the configured logger/stdout for debugging.
type Adapter struct {
	name string
	base adapters.BaseAdapter
	caps adapters.Capability
}

type Option func(*Adapter)

// WithName overrides the adapter provider name (defaults to "console").
func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

// New constructs a console adapter.
func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "console",
		caps: adapters.Capability{
			Name:     "console",
			Channels: []string{"email"},
			Formats:  []string{"text/plain", "text/html"},
		},
	}
	adapter.base = adapters.NewBaseAdapter(l)
	for _, opt := range opts {
		if opt != nil {
			opt(adapter)
		}
	}
	return adapter
}

// Name implements adapters.Messenger.
func (a *Adapter) Name() string {
	return a.name
}

// Capabilities implements adapters.Messenger.
func (a *Adapter) Capabilities() adapters.Capability {
	return a.caps
}

// Send logs the rendered message to the configured logger.
func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
	fmtStr := "[console][%s] subject=%s to=%s body=%s"
	a.base.LogSuccess(a.name, msg)
	a.base.Logger().Info(fmt.Sprintf(fmtStr, msg.Channel, msg.Subject, msg.To, msg.Body))
	return nil
}
