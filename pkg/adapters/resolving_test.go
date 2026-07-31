package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/goliatone/go-notifications/pkg/privacy"
)

func TestResolvingMessengerMutatesOnlyDeliveryCopy(t *testing.T) {
	inner := &captureMessenger{name: "smtp"}
	resolver := staticResolver{result: ResolvedRecipient{Destination: "person@example.com", Locale: "fr"}}
	messenger := ResolvingMessenger{Inner: inner, Resolver: resolver}
	original := Message{ID: "message-1", Channel: "email", To: "subject-123", Locale: "en"}

	if err := messenger.Send(context.Background(), original); err != nil {
		t.Fatalf("send: %v", err)
	}
	if original.To != "subject-123" || original.Locale != "en" {
		t.Fatalf("original message was mutated: %+v", original)
	}
	if inner.message.To != "person@example.com" || inner.message.Locale != "fr" {
		t.Fatalf("resolved copy not delivered: %+v", inner.message)
	}
}

func TestResolvingMessengerReturnsSafeResolverError(t *testing.T) {
	raw := errors.New("directory token leaked for person@example.com")
	messenger := ResolvingMessenger{
		Inner: &captureMessenger{name: "smtp"}, Resolver: staticResolver{err: raw},
		Privacy: privacy.DefaultPolicy{},
	}
	err := messenger.Send(context.Background(), Message{To: "subject-123", Channel: "email"})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || errors.Is(err, raw) {
		t.Fatalf("expected non-wrapping safe error, got %v", err)
	}
}

func TestResolvingMessengerSanitizesInnerDeliveryError(t *testing.T) {
	raw := errors.New("smtp rejected resolved destination person@example.com")
	inner := &captureMessenger{name: "smtp", err: raw}
	diagnostic := &captureDiagnostic{}
	messenger := ResolvingMessenger{
		Inner: inner,
		Resolver: staticResolver{result: ResolvedRecipient{
			Destination: "person@example.com",
		}},
		Privacy: privacy.DefaultPolicy{}, Diagnostic: diagnostic,
	}
	err := messenger.Send(context.Background(), Message{ID: "message-1", To: "subject-123", Channel: "email"})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || errors.Is(err, raw) ||
		strings.Contains(err.Error(), "person@example.com") {
		t.Fatalf("expected non-wrapping safe delivery error, got %v", err)
	}
	if !errors.Is(diagnostic.cause, raw) {
		t.Fatalf("raw error did not reach privileged diagnostic sink: %v", diagnostic.cause)
	}
}

type staticResolver struct {
	result ResolvedRecipient
	err    error
}

func (r staticResolver) Resolve(context.Context, RecipientRequest) (ResolvedRecipient, error) {
	return r.result, r.err
}

type captureMessenger struct {
	name    string
	message Message
	err     error
}

type captureDiagnostic struct {
	cause error
}

func (d *captureDiagnostic) Report(_ context.Context, event privacy.DiagnosticEvent) {
	d.cause = event.Cause
}

func (m *captureMessenger) Name() string { return m.name }
func (m *captureMessenger) Capabilities() Capability {
	return Capability{Name: m.name, Channels: []string{"email"}}
}
func (m *captureMessenger) Send(_ context.Context, message Message) error {
	m.message = message
	return m.err
}
