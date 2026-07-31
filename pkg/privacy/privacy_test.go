package privacy

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultPolicyMasksAddressesAndDropsSensitiveFields(t *testing.T) {
	policy := DefaultPolicy{}
	subject := policy.SafeSubjectID("person@example.com")
	if subject == "person@example.com" || !strings.HasPrefix(subject, "subject:") {
		t.Fatalf("expected masked subject, got %q", subject)
	}
	safe := policy.SafeMetadata(map[string]any{
		"provider": "smtp", "authorization": "secret", "action_url": "https://private",
	})
	if safe["provider"] != "smtp" || safe["authorization"] != nil || safe["action_url"] != nil {
		t.Fatalf("unexpected safe metadata: %#v", safe)
	}
}

func TestSafeErrorDoesNotExposeOrUnwrapCause(t *testing.T) {
	raw := errors.New("smtp rejected person@example.com with token abc")
	safe := DefaultPolicy{}.SafeError(raw)
	if strings.Contains(safe.Error(), "person@example.com") || strings.Contains(safe.Error(), "abc") {
		t.Fatalf("safe error leaked raw cause: %v", safe)
	}
	if errors.Is(safe, raw) {
		t.Fatalf("safe error must not unwrap raw cause")
	}
}
