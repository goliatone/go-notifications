package adapters

import "github.com/goliatone/go-notifications/pkg/interfaces/logger"

// BaseAdapter provides shared helpers for simple adapters.
type BaseAdapter struct {
	logger logger.Logger
}

func NewBaseAdapter(l logger.Logger) BaseAdapter {
	if l == nil {
		l = logger.Default()
	}
	return BaseAdapter{logger: l}
}

func (b BaseAdapter) LogSuccess(name string, msg Message) {
	b.logger.Info("adapter delivered message", "adapter", name, "channel", msg.Channel, "to", msg.To)
}

func (b BaseAdapter) LogFailure(name string, msg Message, err error) {
	b.logger.Error("adapter delivery failed", "adapter", name, "channel", msg.Channel, "to", msg.To, "error", err)
}

// Logger exposes the adapter logger for structured diagnostics.
func (b BaseAdapter) Logger() logger.Logger {
	if b.logger == nil {
		return logger.Default()
	}
	return b.logger
}
