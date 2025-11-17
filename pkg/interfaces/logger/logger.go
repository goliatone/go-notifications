package logger

// Field represents a structured logging key/value pair.
type Field struct {
	Key   string
	Value any
}

// Logger is the minimal contract expected by go-notifications services.
// Implementations may forward to go-logger, zap, logrus, etc.
type Logger interface {
	With(fields ...Field) Logger
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

// Nop is a no-op logger implementation useful for tests.
type Nop struct{}

// Ensure Nop satisfies Logger.
var _ Logger = (*Nop)(nil)

func (n *Nop) With(fields ...Field) Logger       { return n }
func (n *Nop) Debug(msg string, fields ...Field) {}
func (n *Nop) Info(msg string, fields ...Field)  {}
func (n *Nop) Warn(msg string, fields ...Field)  {}
func (n *Nop) Error(msg string, fields ...Field) {}
