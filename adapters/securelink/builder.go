package securelink

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/links"
)

// RouteFunc selects a securelink route for a request.
type RouteFunc func(req links.LinkRequest) string

// PayloadBuilder constructs securelink payloads for a given request.
type PayloadBuilder func(req links.LinkRequest, key string) links.SecureLinkPayload

// Builder produces secure links using a SecureLinkManager.
type Builder struct {
	manager        links.SecureLinkManager
	actionRoute    RouteFunc
	manifestRoute  RouteFunc
	payloadBuilder PayloadBuilder
	now            func() time.Time
}

// Option configures the securelink builder.
type Option func(*Builder)

// NewBuilder creates a securelink-backed LinkBuilder.
func NewBuilder(manager links.SecureLinkManager, opts ...Option) *Builder {
	builder := &Builder{
		manager:        manager,
		actionRoute:    func(links.LinkRequest) string { return "action" },
		payloadBuilder: defaultPayloadBuilder,
		now:            time.Now,
	}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

// WithActionRoute sets a fixed route for action links.
func WithActionRoute(route string) Option {
	return WithActionRouteFunc(func(links.LinkRequest) string { return route })
}

// WithActionRouteFunc sets a route selector for action links.
func WithActionRouteFunc(fn RouteFunc) Option {
	return func(builder *Builder) {
		if fn != nil {
			builder.actionRoute = fn
		}
	}
}

// WithManifestRoute sets a fixed route for manifest links.
func WithManifestRoute(route string) Option {
	return WithManifestRouteFunc(func(links.LinkRequest) string { return route })
}

// WithManifestRouteFunc sets a route selector for manifest links.
func WithManifestRouteFunc(fn RouteFunc) Option {
	return func(builder *Builder) {
		if fn != nil {
			builder.manifestRoute = fn
		}
	}
}

// WithPayloadBuilder overrides the payload builder.
func WithPayloadBuilder(fn PayloadBuilder) Option {
	return func(builder *Builder) {
		if fn != nil {
			builder.payloadBuilder = fn
		}
	}
}

// WithClock overrides the clock used for expiration timestamps.
func WithClock(now func() time.Time) Option {
	return func(builder *Builder) {
		if now != nil {
			builder.now = now
		}
	}
}

// Build generates secure links for the request.
func (b *Builder) Build(ctx context.Context, req links.LinkRequest) (links.ResolvedLinks, error) {
	if b == nil {
		return links.ResolvedLinks{}, errors.New("securelink builder is nil")
	}
	if b.manager == nil {
		return links.ResolvedLinks{}, errors.New("securelink manager is required")
	}
	payloadBuilder := b.payloadBuilder
	if payloadBuilder == nil {
		payloadBuilder = defaultPayloadBuilder
	}
	now := b.now
	if now == nil {
		now = time.Now
	}

	var expiresAt time.Time
	if ttl := b.manager.GetExpiration(); ttl > 0 {
		expiresAt = now().Add(ttl)
	}

	resolved := links.ResolvedLinks{}
	records := make([]links.LinkRecord, 0, 2)

	if route := routeFor(b.actionRoute, req); route != "" {
		url, err := b.manager.Generate(route, payloadBuilder(req, links.ResolvedURLActionKey))
		if err != nil {
			return links.ResolvedLinks{}, err
		}
		resolved.ActionURL = url
		resolved.URL = url
		records = append(records, buildRecord(req, url, links.ResolvedURLActionKey, route, expiresAt))
	}

	if route := routeFor(b.manifestRoute, req); route != "" {
		url, err := b.manager.Generate(route, payloadBuilder(req, links.ResolvedURLManifestKey))
		if err != nil {
			return links.ResolvedLinks{}, err
		}
		resolved.ManifestURL = url
		records = append(records, buildRecord(req, url, links.ResolvedURLManifestKey, route, expiresAt))
	}

	if len(records) > 0 {
		resolved.Records = records
	}
	return resolved, nil
}

func routeFor(fn RouteFunc, req links.LinkRequest) string {
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(fn(req))
}

func defaultPayloadBuilder(req links.LinkRequest, key string) links.SecureLinkPayload {
	return links.SecureLinkPayload{
		"definition": req.Definition,
		"event_id":   req.EventID,
		"channel":    req.Channel,
		"provider":   req.Provider,
		"recipient":  req.Recipient,
		"message_id": req.MessageID,
		"template":   req.TemplateCode,
		"locale":     req.Locale,
		"link_key":   key,
	}
}

func buildRecord(req links.LinkRequest, url, key, route string, expiresAt time.Time) links.LinkRecord {
	metadata := map[string]any{
		"link_key": key,
		"route":    route,
	}
	return links.LinkRecord{
		URL:        url,
		Channel:    req.Channel,
		Recipient:  req.Recipient,
		MessageID:  req.MessageID,
		Definition: req.Definition,
		ExpiresAt:  expiresAt,
		Metadata:   metadata,
	}
}
