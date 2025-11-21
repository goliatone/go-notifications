package secrets

import (
	"bytes"
	"testing"
)

func TestEncryptedStoreProviderRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	store := NewMemoryStore()
	prov, err := NewEncryptedStoreProvider(store, key)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "telegram", Key: "token"}
	ver, err := prov.Put(ref, []byte("supersecret"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if ver == "" {
		t.Fatalf("expected version")
	}

	got, err := prov.Get(ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Data) != "supersecret" {
		t.Fatalf("want supersecret, got %s", got.Data)
	}
	if got.Version != ver {
		t.Fatalf("version mismatch")
	}
}

func TestEncryptedStoreProviderDelete(t *testing.T) {
	key := bytes.Repeat([]byte{2}, 32)
	store := NewMemoryStore()
	prov, err := NewEncryptedStoreProvider(store, key)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "email", Provider: "sendgrid", Key: "api_key"}
	if _, err := prov.Put(ref, []byte("k")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := prov.Delete(ref); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := prov.Get(ref); err == nil {
		t.Fatalf("expected not found after delete")
	}
}
