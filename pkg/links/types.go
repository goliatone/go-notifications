package links

import (
	"context"
	"strings"
	"time"
)

const (
	// ResolvedURLActionKey is the canonical CTA link key.
	ResolvedURLActionKey = "action_url"
	// ResolvedURLManifestKey carries a manifest/download link.
	ResolvedURLManifestKey = "manifest_url"
	// ResolvedURLKey is deprecated in favor of ResolvedURLActionKey.
	ResolvedURLKey = "url"
	// ResolvedURLMetaPrefix namespaces extra resolved URL fields.
	ResolvedURLMetaPrefix = "meta."
)

// ResolvedURLKeySet lists the canonical resolved URL keys.
var ResolvedURLKeySet = map[string]struct{}{
	ResolvedURLActionKey:   {},
	ResolvedURLManifestKey: {},
	ResolvedURLKey:         {},
}

// IsResolvedURLKey reports whether key is a canonical URL key or a namespaced extra.
func IsResolvedURLKey(key string) bool {
	if _, ok := ResolvedURLKeySet[key]; ok {
		return true
	}
	return strings.HasPrefix(key, ResolvedURLMetaPrefix)
}

// LinkBuilder generates resolved links for a notification.
type LinkBuilder interface {
	Build(ctx context.Context, req LinkRequest) (ResolvedLinks, error)
}

// LinkRequest captures the context passed to a LinkBuilder.
type LinkRequest struct {
	EventID      string
	Definition   string
	Recipient    string
	Channel      string
	Provider     string
	TemplateCode string
	MessageID    string
	Locale       string
	Payload      map[string]any // channel-aware payload (after overrides)
	Metadata     map[string]any // message metadata (current state)
	ResolvedURLs map[string]string
}

// ResolvedLinks carries resolved link outputs plus optional metadata/records.
type ResolvedLinks struct {
	ActionURL   string
	ManifestURL string
	URL         string
	Metadata    map[string]any // builder-provided metadata (tokens, expiry, ids)
	Records     []LinkRecord   // optional, for storage/analytics
}

// LinkRecord represents a stored or auditable link resolution record.
type LinkRecord struct {
	ID         string
	URL        string
	Channel    string
	Recipient  string
	MessageID  string
	Definition string
	ExpiresAt  time.Time
	Metadata   map[string]any
}

// LinkStore persists resolved link records.
type LinkStore interface {
	Save(ctx context.Context, records []LinkRecord) error
}

// LinkObserver receives resolved link events.
type LinkObserver interface {
	OnLinksResolved(ctx context.Context, info LinkResolution)
}

// LinkResolution bundles the request and resolved outputs.
type LinkResolution struct {
	Request  LinkRequest
	Resolved ResolvedLinks
}

// FailureMode controls how link resolution errors are handled.
type FailureMode string

const (
	// FailureStrict aborts processing on error.
	FailureStrict FailureMode = "strict"
	// FailureLenient logs and continues on error.
	FailureLenient FailureMode = "lenient"
)

// FailurePolicy configures failure handling per dependency.
type FailurePolicy struct {
	Builder  FailureMode
	Store    FailureMode
	Observer FailureMode
}
