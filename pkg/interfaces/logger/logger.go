package logger

import "context"

// Logger matches the go-logger contract so external implementations can be used directly.
type Logger interface {
	Trace(msg string, args ...any)
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Fatal(msg string, args ...any)
	WithContext(ctx context.Context) Logger
}

// LoggerProvider exposes named logger retrieval (mirrors go-logger).
type LoggerProvider interface {
	GetLogger(name string) Logger
}

// FieldsLogger is an optional extension for attaching structured fields.
type FieldsLogger interface {
	WithFields(map[string]any) Logger
}

// Nop is a no-op logger implementation useful for tests.
type Nop struct{}

// Ensure Nop satisfies Logger.
var _ Logger = (*Nop)(nil)

func (n *Nop) Trace(msg string, args ...any) {}
func (n *Nop) Debug(msg string, args ...any) {}
func (n *Nop) Info(msg string, args ...any)  {}
func (n *Nop) Warn(msg string, args ...any)  {}
func (n *Nop) Error(msg string, args ...any) {}
func (n *Nop) Fatal(msg string, args ...any) {}
func (n *Nop) WithContext(ctx context.Context) Logger {
	return n
}
