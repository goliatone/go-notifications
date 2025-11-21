package secrets

import "testing"

func TestStaticProviderRoundTrip(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	p := NewStaticProvider(nil)
	ver, err := p.Put(ref, []byte("secret"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if ver == "" {
		t.Fatalf("expected version to be set")
	}
	val, err := p.Get(ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(val.Data) != "secret" {
		t.Fatalf("expected secret, got %s", val.Data)
	}
	if val.Retrieved.IsZero() {
		t.Fatalf("expected retrieved timestamp")
	}
}

func TestMaskValues(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	masked := MaskValues(map[Reference]SecretValue{
		ref: {Data: []byte("abcd1234"), Version: "v1"},
	})
	if len(masked) != 1 {
		t.Fatalf("expected 1 masked entry")
	}
	for _, v := range masked {
		entry := v.(map[string]any)
		if entry["value"] == "abcd1234" {
			t.Fatalf("expected value to be masked")
		}
	}
}
