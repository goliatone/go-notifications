package secrets

import "testing"

func TestNopProvider(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	var p NopProvider
	if _, err := p.Get(ref); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound")
	}
	if err := p.Delete(ref); err != ErrUnsupported {
		t.Fatalf("expected ErrUnsupported on delete")
	}
}
