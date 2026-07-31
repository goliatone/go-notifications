package adapters

import (
	"context"
	"errors"
	"maps"

	"github.com/goliatone/go-notifications/pkg/privacy"
)

type RecipientRequest struct {
	SubjectID string
	Channel   string
	Provider  string
}

type ResolvedRecipient struct {
	Destination string
	Locale      string
	Metadata    map[string]any
}

type RecipientResolver interface {
	Resolve(context.Context, RecipientRequest) (ResolvedRecipient, error)
}

// ResolvingMessenger resolves an opaque subject immediately before delivery
// and mutates only a copy of the adapter message.
type ResolvingMessenger struct {
	Inner      Messenger
	Resolver   RecipientResolver
	Privacy    privacy.Policy
	Diagnostic privacy.DiagnosticSink
}

func (m ResolvingMessenger) Name() string {
	if m.Inner == nil {
		return ""
	}
	return m.Inner.Name()
}

func (m ResolvingMessenger) Capabilities() Capability {
	if m.Inner == nil {
		return Capability{}
	}
	return m.Inner.Capabilities()
}

func (m ResolvingMessenger) Send(ctx context.Context, msg Message) error {
	if m.Inner == nil || m.Resolver == nil {
		return privacy.SafeError{Category: "configuration", Code: "recipient_resolver_unavailable", Message: "recipient resolver unavailable"}
	}
	resolved, err := m.Resolver.Resolve(ctx, RecipientRequest{
		SubjectID: msg.To,
		Channel:   msg.Channel,
		Provider:  msg.Provider,
	})
	if err != nil || resolved.Destination == "" {
		if m.Diagnostic != nil && err != nil {
			m.Diagnostic.Report(ctx, privacy.DiagnosticEvent{Operation: "recipient.resolve", MessageID: msg.ID, Cause: err})
		}
		if err == nil {
			err = errors.New("empty recipient destination")
		}
		policy := m.Privacy
		if policy == nil {
			policy = privacy.DefaultPolicy{}
		}
		return policy.SafeError(err)
	}
	copyMessage := msg
	copyMessage.To = resolved.Destination
	copyMessage.Metadata = maps.Clone(msg.Metadata)
	if len(resolved.Metadata) > 0 {
		if copyMessage.Metadata == nil {
			copyMessage.Metadata = make(map[string]any, len(resolved.Metadata))
		}
		maps.Copy(copyMessage.Metadata, resolved.Metadata)
	}
	if resolved.Locale != "" {
		copyMessage.Locale = resolved.Locale
	}
	if err := m.Inner.Send(ctx, copyMessage); err != nil {
		if m.Diagnostic != nil {
			m.Diagnostic.Report(ctx, privacy.DiagnosticEvent{
				Operation: "recipient.deliver",
				MessageID: msg.ID,
				Cause:     err,
			})
		}
		policy := m.Privacy
		if policy == nil {
			policy = privacy.DefaultPolicy{}
		}
		return policy.SafeError(err)
	}
	return nil
}
