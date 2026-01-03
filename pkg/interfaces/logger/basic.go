package logger

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// BasicLogger prints log lines using fmt.Printf.
type BasicLogger struct {
	mu     *sync.Mutex
	ctx    context.Context
	fields map[string]any
}

var _ Logger = (*BasicLogger)(nil)
var _ FieldsLogger = (*BasicLogger)(nil)

// New returns a basic logger that writes to stdout using fmt.Printf.
func New() *BasicLogger {
	return &BasicLogger{
		mu:     &sync.Mutex{},
		ctx:    context.Background(),
		fields: make(map[string]any),
	}
}

// Default returns the default basic logger implementation.
func Default() Logger {
	return New()
}

// WithFields returns a logger that includes structured fields on each log line.
func (l *BasicLogger) WithFields(fields map[string]any) Logger {
	if len(fields) == 0 {
		return l
	}
	next := l.clone()
	for k, v := range fields {
		next.fields[k] = v
	}
	return next
}

func (l *BasicLogger) WithContext(ctx context.Context) Logger {
	if ctx == nil {
		return l
	}
	next := l.clone()
	next.ctx = ctx
	return next
}

func (l *BasicLogger) Trace(msg string, args ...any) { l.log("TRACE", msg, args...) }
func (l *BasicLogger) Debug(msg string, args ...any) { l.log("DEBUG", msg, args...) }
func (l *BasicLogger) Info(msg string, args ...any)  { l.log("INFO", msg, args...) }
func (l *BasicLogger) Warn(msg string, args ...any)  { l.log("WARN", msg, args...) }
func (l *BasicLogger) Error(msg string, args ...any) { l.log("ERROR", msg, args...) }

func (l *BasicLogger) Fatal(msg string, args ...any) {
	l.log("FATAL", msg, args...)
	os.Exit(1)
}

func (l *BasicLogger) log(level, msg string, args ...any) {
	allArgs := append(fieldArgs(l.fields), args...)
	line := fmt.Sprintf("[%s] %s", level, msg)
	if rendered := formatArgs(allArgs); rendered != "" {
		line += " " + rendered
	}
	l.mu.Lock()
	fmt.Printf("%s\n", line)
	l.mu.Unlock()
}

func (l *BasicLogger) clone() *BasicLogger {
	out := &BasicLogger{
		mu:     l.mu,
		ctx:    l.ctx,
		fields: make(map[string]any, len(l.fields)),
	}
	for k, v := range l.fields {
		out.fields[k] = v
	}
	return out
}

func fieldArgs(fields map[string]any) []any {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		args = append(args, k, fields[k])
	}
	return args
}

func formatArgs(args []any) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for i := 0; i < len(args); {
		if key, ok := args[i].(string); ok && i+1 < len(args) {
			parts = append(parts, fmt.Sprintf("%s=%s", key, fmt.Sprint(args[i+1])))
			i += 2
			continue
		}
		parts = append(parts, fmt.Sprint(args[i]))
		i++
	}
	return strings.Join(parts, " ")
}
