package main

import (
	"fmt"

	appLogger "github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-router"
)

type routerLoggerAdapter struct {
	base appLogger.Logger
}

var _ router.Logger = (*routerLoggerAdapter)(nil)

func newRouterLogger(base appLogger.Logger) router.Logger {
	if base == nil {
		return nil
	}
	return &routerLoggerAdapter{base: base}
}

func (l *routerLoggerAdapter) Debug(format string, args ...any) {
	l.log(l.base.Debug, format, args...)
}

func (l *routerLoggerAdapter) Info(format string, args ...any) {
	l.log(l.base.Info, format, args...)
}

func (l *routerLoggerAdapter) Warn(format string, args ...any) {
	l.log(l.base.Warn, format, args...)
}

func (l *routerLoggerAdapter) Error(format string, args ...any) {
	l.log(l.base.Error, format, args...)
}

func (l *routerLoggerAdapter) log(fn func(string, ...appLogger.Field), format string, args ...any) {
	if l == nil || l.base == nil || fn == nil {
		return
	}

	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	fn(msg)
}
