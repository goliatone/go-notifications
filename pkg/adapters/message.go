package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Message represents a rendered notification destined for a single channel/provider combo.
type Message struct {
	ID          string
	Channel     string
	Provider    string
	Subject     string
	Body        string
	To          string
	Attachments []Attachment
	Metadata    map[string]any
	Locale      string
	Headers     map[string]string
	Attempts    int
	TraceID     string
	RequestID   string
}

// Capability describes the channels/formats supported by a messenger.
type Capability struct {
	Name           string
	Channels       []string
	Formats        []string
	MaxAttachments int
	Metadata       map[string]string
}

// Messenger is implemented by channel adapters (SMTP, Twilio, etc).
type Messenger interface {
	Name() string
	Capabilities() Capability
	Send(ctx context.Context, msg Message) error
}

// ErrAdapterNotFound is returned when no messenger can satisfy a route.
var ErrAdapterNotFound = errors.New("adapters: no adapter matches route")

// Registry stores available messengers and matches channels to providers.
type Registry struct {
	mu        sync.RWMutex
	adapters  map[string]Messenger
	byChannel map[string][]Messenger
}

// NewRegistry builds a registry with the supplied messengers.
func NewRegistry(messengers ...Messenger) *Registry {
	reg := &Registry{
		adapters:  make(map[string]Messenger),
		byChannel: make(map[string][]Messenger),
	}
	for _, m := range messengers {
		reg.Register(m)
	}
	return reg
}

// Register adds a messenger, indexing by provider name and supported channels.
func (r *Registry) Register(m Messenger) {
	if r == nil || m == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	name := normalizeKey(m.Name())
	if name != "" {
		r.adapters[name] = m
	}
	for _, channel := range m.Capabilities().Channels {
		key := normalizeKey(channel)
		if key == "" {
			continue
		}
		r.byChannel[key] = append(r.byChannel[key], m)
	}
}

// Route locates a messenger based on channel string (e.g., email:console).
func (r *Registry) Route(channel string) (Messenger, error) {
	if r == nil {
		return nil, ErrAdapterNotFound
	}
	ch, provider := ParseChannel(channel)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if provider != "" {
		if adapter, ok := r.adapters[provider]; ok {
			return adapter, nil
		}
		return nil, ErrAdapterNotFound
	}
	candidates := r.byChannel[normalizeKey(ch)]
	if len(candidates) == 0 {
		return nil, ErrAdapterNotFound
	}
	return candidates[0], nil
}

// List returns all messengers registered for a logical channel.
func (r *Registry) List(channel string) []Messenger {
	if r == nil {
		return nil
	}
	base, _ := ParseChannel(channel)
	r.mu.RLock()
	defer r.mu.RUnlock()
	candidates := r.byChannel[normalizeKey(channel)]
	if len(candidates) == 0 && base != normalizeKey(channel) {
		candidates = r.byChannel[normalizeKey(base)]
	}
	out := make([]Messenger, len(candidates))
	copy(out, candidates)
	return out
}

// ParseChannel splits "<channel>[:provider]" into components.
func ParseChannel(value string) (channel string, provider string) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return strings.ToLower(parts[0]), ""
	default:
		return strings.ToLower(parts[0]), normalizeKey(parts[1])
	}
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// Describe returns a human-readable summary of the registry entries.
func (r *Registry) Describe() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.adapters))
	for name, adapter := range r.adapters {
		caps := adapter.Capabilities()
		out = append(out, fmt.Sprintf("%s (%s)", name, strings.Join(caps.Channels, ",")))
	}
	return out
}
