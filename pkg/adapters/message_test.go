package adapters

import (
	"context"
	"testing"
)

func TestRegistryProviderQualifiedRoutesRemainProviderScoped(t *testing.T) {
	smtp := &registryMessenger{name: "smtp", channels: []string{"email"}}
	console := &registryMessenger{name: "console", channels: []string{"email"}}
	sms := &registryMessenger{name: "sms-only", channels: []string{"sms"}}
	registry := NewRegistry(smtp, console, sms)

	listed := registry.List("email:console")
	if len(listed) != 1 || listed[0].Name() != "console" {
		t.Fatalf("provider-qualified list escaped its provider: %+v", listed)
	}
	if listed := registry.List("email:sms-only"); len(listed) != 0 {
		t.Fatalf("provider without channel capability was returned: %+v", listed)
	}
	routed, err := registry.Route("email:console")
	if err != nil || routed.Name() != "console" {
		t.Fatalf("provider-qualified route failed: route=%v err=%v", routed, err)
	}
	if _, err := registry.Route("email:sms-only"); err == nil {
		t.Fatalf("provider without channel capability was routed")
	}
}

type registryMessenger struct {
	name     string
	channels []string
}

func (m *registryMessenger) Name() string { return m.name }
func (m *registryMessenger) Capabilities() Capability {
	return Capability{Name: m.name, Channels: m.channels}
}
func (*registryMessenger) Send(context.Context, Message) error { return nil }
