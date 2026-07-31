package adapters

import (
	"strings"
	"testing"
)

func TestHTTPStatusErrorOmitsProviderResponseBody(t *testing.T) {
	marker := "provider response leaked person@example.com token-123"
	err := HTTPStatusError("test", 401, []byte(marker))
	if strings.Contains(err.Error(), marker) ||
		strings.Contains(err.Error(), "person@example.com") ||
		strings.Contains(err.Error(), "token-123") {
		t.Fatalf("HTTP status error leaked provider response: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("HTTP status error omitted safe status code: %v", err)
	}
}
