package securelink

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/links"
)

func TestBuilderBuildActionOnlyWithAdapter(t *testing.T) {
	manager := WrapManager(newTestURLKitManager(t))
	fixedNow := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	builder := NewBuilder(
		manager,
		WithManifestRoute(""),
		WithClock(func() time.Time { return fixedNow }),
	)

	req := testLinkRequest()
	resolved, err := builder.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("expected resolved links, got error: %v", err)
	}

	if resolved.ActionURL == "" {
		t.Fatal("expected action url")
	}
	if resolved.URL != resolved.ActionURL {
		t.Fatalf("expected URL alias to action URL, got %q vs %q", resolved.URL, resolved.ActionURL)
	}
	if resolved.ManifestURL != "" {
		t.Fatalf("expected empty manifest url, got %q", resolved.ManifestURL)
	}
	if len(resolved.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resolved.Records))
	}

	record := resolved.Records[0]
	if got := asString(record.Metadata["route"]); got != "action" {
		t.Fatalf("expected record route action, got %q", got)
	}
	if got := asString(record.Metadata["link_key"]); got != links.ResolvedURLActionKey {
		t.Fatalf("expected link_key %q, got %q", links.ResolvedURLActionKey, got)
	}

	expectedExpiresAt := fixedNow.Add(manager.GetExpiration())
	if !record.ExpiresAt.Equal(expectedExpiresAt) {
		t.Fatalf("expected ExpiresAt %v, got %v", expectedExpiresAt, record.ExpiresAt)
	}

	token := extractTokenFromURL(t, resolved.ActionURL, "token")
	payload, err := manager.Validate(token)
	if err != nil {
		t.Fatalf("validate generated token: %v", err)
	}
	if got := asString(payload["link_key"]); got != links.ResolvedURLActionKey {
		t.Fatalf("expected payload link_key %q, got %q", links.ResolvedURLActionKey, got)
	}
	if got := asString(payload["definition"]); got != req.Definition {
		t.Fatalf("expected payload definition %q, got %q", req.Definition, got)
	}
}

func TestBuilderBuildActionAndManifestWithAdapter(t *testing.T) {
	manager := WrapManager(newTestURLKitManager(t))
	builder := NewBuilder(
		manager,
		WithActionRoute("action"),
		WithManifestRoute("manifest"),
	)

	req := testLinkRequest()
	resolved, err := builder.Build(context.Background(), req)
	if err != nil {
		t.Fatalf("expected resolved links, got error: %v", err)
	}

	if resolved.ActionURL == "" {
		t.Fatal("expected action url")
	}
	if resolved.ManifestURL == "" {
		t.Fatal("expected manifest url")
	}
	if len(resolved.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(resolved.Records))
	}

	actionToken := extractTokenFromURL(t, resolved.ActionURL, "token")
	actionPayload, err := manager.Validate(actionToken)
	if err != nil {
		t.Fatalf("validate action token: %v", err)
	}
	if got := asString(actionPayload["link_key"]); got != links.ResolvedURLActionKey {
		t.Fatalf("expected action payload key %q, got %q", links.ResolvedURLActionKey, got)
	}

	manifestToken := extractTokenFromURL(t, resolved.ManifestURL, "token")
	manifestPayload, err := manager.Validate(manifestToken)
	if err != nil {
		t.Fatalf("validate manifest token: %v", err)
	}
	if got := asString(manifestPayload["link_key"]); got != links.ResolvedURLManifestKey {
		t.Fatalf("expected manifest payload key %q, got %q", links.ResolvedURLManifestKey, got)
	}
}

func TestBuilderSkipsEmptyRoutes(t *testing.T) {
	manager := WrapManager(newTestURLKitManager(t))
	builder := NewBuilder(
		manager,
		WithActionRoute(""),
		WithManifestRoute(" "),
	)

	resolved, err := builder.Build(context.Background(), testLinkRequest())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resolved.ActionURL != "" || resolved.ManifestURL != "" || resolved.URL != "" {
		t.Fatalf("expected empty resolved links, got %+v", resolved)
	}
	if len(resolved.Records) != 0 {
		t.Fatalf("expected no records, got %d", len(resolved.Records))
	}
}

func TestBuilderReturnsErrorWhenActionRouteMissing(t *testing.T) {
	manager := WrapManager(newTestURLKitManager(t))
	builder := NewBuilder(
		manager,
		WithActionRoute("missing-route"),
		WithManifestRoute(""),
	)

	_, err := builder.Build(context.Background(), testLinkRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing-route") {
		t.Fatalf("expected route error mentioning missing-route, got %v", err)
	}
}

func TestBuilderReturnsErrorWhenManifestRouteMissing(t *testing.T) {
	manager := WrapManager(newTestURLKitManager(t))
	builder := NewBuilder(
		manager,
		WithActionRoute("action"),
		WithManifestRoute("missing-route"),
	)

	_, err := builder.Build(context.Background(), testLinkRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing-route") {
		t.Fatalf("expected route error mentioning missing-route, got %v", err)
	}
}

func TestBuilderNilReceiver(t *testing.T) {
	var builder *Builder

	_, err := builder.Build(context.Background(), testLinkRequest())
	if err == nil {
		t.Fatal("expected nil builder error")
	}
}

func TestBuilderRequiresManager(t *testing.T) {
	builder := NewBuilder(nil)

	_, err := builder.Build(context.Background(), testLinkRequest())
	if err == nil {
		t.Fatal("expected missing manager error")
	}
}

func testLinkRequest() links.LinkRequest {
	return links.LinkRequest{
		EventID:      "evt_1",
		Definition:   "users.password_reset",
		Recipient:    "user@example.com",
		Channel:      "email",
		Provider:     "smtp",
		TemplateCode: "users.password_reset",
		MessageID:    "msg_1",
		Locale:       "en",
	}
}
