package adapters

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport == nil {
		transport = &http.Transport{}
	}
	cloned := transport.Clone()
	if cfg.normalizedTLSPolicy() == TLSPolicyInsecureSkipVerify {
		// #nosec G402 -- callers must explicitly opt into this documented
		// compatibility mode; strict certificate verification is the default.
		cloned.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: cloned,
	}
}

// ReadResponseBody drains an HTTP response body. The caller remains responsible
// for closing the body so ownership stays explicit at the request boundary.
func ReadResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("adapters: response body is not available")
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("adapters: read response body: %w", err)
	}
	return data, nil
}

// ExecuteRequest executes an adapter HTTP request and owns response cleanup,
// response-body reading, and non-success status handling.
func ExecuteRequest(client *http.Client, adapter string, req *http.Request) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("%s: HTTP client is required", adapter)
	}
	if req == nil || req.URL == nil || req.URL.Hostname() == "" {
		return nil, fmt.Errorf("%s: valid request URL is required", adapter)
	}
	if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
		return nil, fmt.Errorf("%s: unsupported request URL scheme %q", adapter, req.URL.Scheme)
	}
	// #nosec G704 -- provider endpoints come from trusted operator
	// configuration; recipient and template content cannot select this URL.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", adapter, err)
	}
	body, readErr := ReadResponseBody(resp)
	closeErr := resp.Body.Close()
	if responseErr := errors.Join(readErr, closeErr); responseErr != nil {
		return nil, fmt.Errorf("%s: %w", adapter, responseErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, HTTPStatusError(adapter, resp.StatusCode, body)
	}
	return body, nil
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

// HTTPStatusError standardizes non-2xx errors without exposing provider
// response bodies, which may contain destinations, credentials, or content.
func HTTPStatusError(adapter string, statusCode int, _ []byte) error {
	return fmt.Errorf("%s: unexpected status %d", adapter, statusCode)
}
