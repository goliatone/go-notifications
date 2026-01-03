package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// stdLogger is a lightweight logger that forwards to the standard library logger.
type stdLogger struct{}

func (l *stdLogger) Trace(msg string, args ...any) { l.log("TRACE", msg, args...) }
func (l *stdLogger) Debug(msg string, args ...any) { l.log("DEBUG", msg, args...) }
func (l *stdLogger) Info(msg string, args ...any)  { l.log("INFO", msg, args...) }
func (l *stdLogger) Warn(msg string, args ...any)  { l.log("WARN", msg, args...) }
func (l *stdLogger) Error(msg string, args ...any) { l.log("ERROR", msg, args...) }
func (l *stdLogger) Fatal(msg string, args ...any) { l.log("FATAL", msg, args...) }
func (l *stdLogger) WithContext(ctx context.Context) logger.Logger {
	return l
}

func (l *stdLogger) log(level, msg string, args ...any) {
	var parts []string
	for i := 0; i < len(args); {
		if key, ok := args[i].(string); ok && i+1 < len(args) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, args[i+1]))
			i += 2
			continue
		}
		parts = append(parts, fmt.Sprint(args[i]))
		i++
	}
	payload := strings.TrimSpace(strings.Join(parts, " "))
	if payload != "" {
		log.Printf("[%s] %s %s", level, msg, payload)
		return
	}
	log.Printf("[%s] %s", level, msg)
}

func newStdLogger() logger.Logger {
	return &stdLogger{}
}
