package main

import (
	"log"

	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

// stdLogger is a lightweight logger that forwards to the standard library logger.
type stdLogger struct{}

func (l *stdLogger) With(fields ...logger.Field) logger.Logger { return l }
func (l *stdLogger) Debug(msg string, fields ...logger.Field) {
	log.Printf("[DEBUG] %s %v", msg, fields)
}
func (l *stdLogger) Info(msg string, fields ...logger.Field) { log.Printf("[INFO] %s %v", msg, fields) }
func (l *stdLogger) Warn(msg string, fields ...logger.Field) { log.Printf("[WARN] %s %v", msg, fields) }
func (l *stdLogger) Error(msg string, fields ...logger.Field) {
	log.Printf("[ERROR] %s %v", msg, fields)
}

func newStdLogger() logger.Logger {
	return &stdLogger{}
}
