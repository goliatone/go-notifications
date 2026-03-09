package adapters

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TLSPolicy controls certificate verification behavior for outbound HTTP clients.
type TLSPolicy string

const (
	// TLSPolicyStrict verifies certificates using system trust roots.
	TLSPolicyStrict TLSPolicy = "strict"
	// TLSPolicyInsecureSkipVerify disables certificate validation (unsafe).
	TLSPolicyInsecureSkipVerify TLSPolicy = "insecure_skip_verify"
)

// HTTPTransportConfig configures common HTTP transport behavior across adapters.
type HTTPTransportConfig struct {
	TLSPolicy TLSPolicy
}

// NewHTTPClient builds a standard HTTP client using the provided timeout and transport policy.
func NewHTTPClient(timeout time.Duration, cfg HTTPTransportConfig) *http.Client {
	transport, _ := http.DefaultTransport.(*http.Transport)
	if transport == nil {
		transport = &http.Transport{}
	}
	cloned := transport.Clone()
	if cfg.normalizedTLSPolicy() == TLSPolicyInsecureSkipVerify {
		cloned.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: cloned,
	}
}

func (cfg HTTPTransportConfig) normalizedTLSPolicy() TLSPolicy {
	switch cfg.TLSPolicy {
	case "", TLSPolicyStrict:
		return TLSPolicyStrict
	case TLSPolicyInsecureSkipVerify:
		return TLSPolicyInsecureSkipVerify
	default:
		return TLSPolicyStrict
	}
}

// PayloadEncodeError wraps JSON payload encoding failures.
type PayloadEncodeError struct {
	Adapter string
	Err     error
}

func (e *PayloadEncodeError) Error() string {
	if e == nil {
		return "payload encode failed"
	}
	adapter := strings.TrimSpace(e.Adapter)
	if adapter == "" {
		adapter = "adapter"
	}
	return fmt.Sprintf("%s: encode payload: %v", adapter, e.Err)
}

func (e *PayloadEncodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrPayloadEncode can be matched for payload serialization failures.
var ErrPayloadEncode = errors.New("adapters: payload encode failed")

// EncodeJSONPayload serializes a payload and returns a typed error on failure.
func EncodeJSONPayload(adapter string, payload any) ([]byte, error) {
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPayloadEncode, &PayloadEncodeError{
			Adapter: adapter,
			Err:     err,
		})
	}
	return out, nil
}

// HTTPStatusError standardizes non-2xx errors including response text when available.
func HTTPStatusError(adapter string, statusCode int, body []byte) error {
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		return fmt.Errorf("%s: unexpected status %d", adapter, statusCode)
	}
	if len(bodyText) > 512 {
		bodyText = bodyText[:512]
	}
	return fmt.Errorf("%s: unexpected status %d: %s", adapter, statusCode, bodyText)
}
