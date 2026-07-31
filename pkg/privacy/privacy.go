package privacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
)

// SafeError is suitable for public return values and persistence. It
// intentionally does not expose or unwrap a raw cause.
type SafeError struct {
	Category string `json:"category"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func (e SafeError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Policy sanitizes identifiers, metadata, context, and errors.
type Policy interface {
	SafeSubjectID(string) string
	SafeContext(map[string]any) map[string]any
	SafeMetadata(map[string]any) map[string]any
	SafeError(error) SafeError
}

// DefaultPolicy preserves opaque IDs while masking address-like subjects and
// removes fields whose names commonly carry content or credentials.
type DefaultPolicy struct{}

func (DefaultPolicy) SafeSubjectID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "@") || looksLikePhone(value) {
		sum := sha256.Sum256([]byte(value))
		return fmt.Sprintf("subject:%x", sum[:8])
	}
	return value
}

func (DefaultPolicy) SafeContext(value map[string]any) map[string]any {
	return safeMap(value)
}

func (DefaultPolicy) SafeMetadata(value map[string]any) map[string]any {
	return safeMap(value)
}

func (DefaultPolicy) SafeError(err error) SafeError {
	if err == nil {
		return SafeError{}
	}
	var safeValue SafeError
	if errors.As(err, &safeValue) {
		return safeValue
	}
	var safePointer *SafeError
	if errors.As(err, &safePointer) && safePointer != nil {
		return *safePointer
	}
	return SafeError{
		Category: "delivery",
		Code:     "notification_delivery_failed",
		Message:  "notification delivery failed",
	}
}

// DiagnosticEvent is delivered only to an explicitly trusted sink.
type DiagnosticEvent struct {
	Operation string
	EventID   string
	MessageID string
	Cause     error
}

// DiagnosticSink receives raw in-memory diagnostics. It is disabled by
// default and is not part of public error or activity paths.
type DiagnosticSink interface {
	Report(context.Context, DiagnosticEvent)
}

type NopDiagnosticSink struct{}

func (NopDiagnosticSink) Report(context.Context, DiagnosticEvent) {}

func safeMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		if sensitiveKey(key) {
			continue
		}
		out[key] = safeValue(value)
	}
	return out
}

func safeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return safeMap(typed)
	case domain.JSONMap:
		return safeMap(map[string]any(typed))
	case map[string]string:
		return safeStringMap(typed)
	case []map[string]any:
		return safeMapSlice(typed)
	case []domain.JSONMap:
		return safeJSONMapSlice(typed)
	case []any:
		return safeValueSlice(typed)
	case []string:
		return safeStringSlice(typed)
	case string:
		return safeString(typed)
	default:
		return value
	}
}

func safeStringMap(src map[string]string) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		if !sensitiveKey(key) {
			out[key] = safeValue(value)
		}
	}
	return out
}

func safeMapSlice(src []map[string]any) []map[string]any {
	out := make([]map[string]any, len(src))
	for idx := range src {
		out[idx] = safeMap(src[idx])
	}
	return out
}

func safeJSONMapSlice(src []domain.JSONMap) []map[string]any {
	out := make([]map[string]any, len(src))
	for idx := range src {
		out[idx] = safeMap(map[string]any(src[idx]))
	}
	return out
}

func safeValueSlice(src []any) []any {
	out := make([]any, len(src))
	for idx := range src {
		out[idx] = safeValue(src[idx])
	}
	return out
}

func safeStringSlice(src []string) []any {
	out := make([]any, len(src))
	for idx := range src {
		out[idx] = safeValue(src[idx])
	}
	return out
}

func safeString(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.Contains(value, "@") ||
		looksLikePhone(value) {
		return "[redacted]"
	}
	return value
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{
		"token", "secret", "password", "credential", "authorization",
		"body", "subject", "html", "url", "recipient", "destination", "to",
	} {
		if key == fragment || strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func looksLikePhone(value string) bool {
	digits := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+', r == '-', r == ' ', r == '(', r == ')':
		default:
			return false
		}
	}
	return digits >= 7
}
