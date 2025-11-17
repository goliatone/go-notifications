package whatsapp

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter simulates WhatsApp sends (chat channel).
type Adapter struct {
	name   string
	base   adapters.BaseAdapter
	caps   adapters.Capability
	number string
}

type Option func(*Adapter)

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func WithBusinessNumber(num string) Option {
	return func(a *Adapter) {
		a.number = num
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "whatsapp",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "whatsapp",
			Channels: []string{"chat"},
			Formats:  []string{"text/plain"},
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
	a.base.Logger().Info(fmt.Sprintf("[whatsapp:%s] to=%s body=%s", a.number, msg.To, msg.Body))
	return nil
}
