package securelink

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/links"
	urlkit "github.com/goliatone/go-urlkit/securelink"
)

var _ links.SecureLinkManager = (*Manager)(nil)

func TestWrapManagerInteropCompileTime(t *testing.T) {
	raw := newTestURLKitManager(t)

	var manager links.SecureLinkManager = WrapManager(raw)
	if manager == nil {
		t.Fatal("expected wrapped manager")
	}
}

func TestNewManagerWithConfigurator(t *testing.T) {
	cfg := testURLKitConfig()

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("expected manager, got error: %v", err)
	}
	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewManagerWithNilConfigurator(t *testing.T) {
	manager, err := NewManager(nil)
	if err == nil {
		t.Fatal("expected error for nil configurator")
	}
	if manager != nil {
		t.Fatal("expected nil manager")
	}
}

func TestWrapManagerNil(t *testing.T) {
	if got := WrapManager(nil); got != nil {
		t.Fatal("expected nil wrapped manager")
	}
}

func TestManagerGenerateValidateAndGetAndValidate(t *testing.T) {
	raw := newTestURLKitManager(t)
	manager := WrapManager(raw)

	link, err := manager.Generate("action", links.SecureLinkPayload{
		"user_id": "user-1",
		"scope":   "password-reset",
	})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	token := extractTokenFromURL(t, link, "token")
	payload, err := manager.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if got := asString(payload["user_id"]); got != "user-1" {
		t.Fatalf("expected user_id user-1, got %q", got)
	}
	if got := asString(payload["scope"]); got != "password-reset" {
		t.Fatalf("expected scope password-reset, got %q", got)
	}

	payload2, err := manager.GetAndValidate(func(key string) string {
		if key != "token" {
			return ""
		}
		return token
	})
	if err != nil {
		t.Fatalf("GetAndValidate failed: %v", err)
	}
	if got := asString(payload2["user_id"]); got != "user-1" {
		t.Fatalf("expected user_id user-1, got %q", got)
	}
}

func TestManagerNilSafety(t *testing.T) {
	var manager *Manager

	if _, err := manager.Generate("action"); err == nil {
		t.Fatal("expected generate error for nil manager")
	}
	if _, err := manager.Validate("token"); err == nil {
		t.Fatal("expected validate error for nil manager")
	}
	if _, err := manager.GetAndValidate(func(string) string { return "" }); err == nil {
		t.Fatal("expected GetAndValidate error for nil manager")
	}
	if got := manager.GetExpiration(); got != 0 {
		t.Fatalf("expected zero expiration, got %v", got)
	}
}

func newTestURLKitManager(t *testing.T) urlkit.Manager {
	t.Helper()

	manager, err := urlkit.NewManager(testURLKitConfig())
	if err != nil {
		t.Fatalf("failed creating urlkit manager: %v", err)
	}
	return manager
}

func testURLKitConfig() urlkit.Config {
	return urlkit.Config{
		SigningKey: "0123456789abcdef0123456789abcdef",
		Expiration: 45 * time.Minute,
		BaseURL:    "https://example.com",
		QueryKey:   "token",
		AsQuery:    true,
		Routes: map[string]string{
			"action":   "/action",
			"manifest": "/manifest",
		},
	}
}

func extractTokenFromURL(t *testing.T, rawURL, queryKey string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse link: %v", err)
	}

	token := parsed.Query().Get(queryKey)
	if token != "" {
		return token
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 {
		t.Fatalf("token not found in URL: %s", rawURL)
	}

	return segments[len(segments)-1]
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}
