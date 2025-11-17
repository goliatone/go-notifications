package telegram

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// Adapter simulates Telegram bot sends.
type Adapter struct {
	name string
	base adapters.BaseAdapter
	caps adapters.Capability
	bot  string
}

type Option func(*Adapter)

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func WithBot(bot string) Option {
	return func(a *Adapter) {
		a.bot = bot
	}
}

func New(l logger.Logger, opts ...Option) *Adapter {
	adapter := &Adapter{
		name: "telegram",
		base: adapters.NewBaseAdapter(l),
		caps: adapters.Capability{
			Name:     "telegram",
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
	a.base.Logger().Info(fmt.Sprintf("[telegram:%s] to=%s body=%s", a.bot, msg.To, msg.Body))
	return nil
}
