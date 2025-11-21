package secrets

import (
	"strings"
	"testing"
)

func TestMaskValuesMasksSecretsAndPreservesVersion(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "user-1", Channel: "chat", Provider: "slack", Key: "token"}
	refWithoutKey := Reference{Scope: ScopeTenant, SubjectID: "tenant-1", Channel: "chat", Provider: "telegram"}

	values := map[Reference]SecretValue{
		ref:           {Data: []byte("supersecretvalue"), Version: "v1"},
		refWithoutKey: {Data: []byte("abcd1234"), Version: "v2"},
	}

	masked := MaskValues(values)
	if len(masked) != 2 {
		t.Fatalf("expected 2 masked entries, got %d", len(masked))
	}

	entry, ok := masked["token"].(map[string]any)
	if !ok {
		t.Fatalf("expected token entry to be present")
	}
	if entry["version"] != "v1" {
		t.Fatalf("expected version v1, got %v", entry["version"])
	}
	if maskedValue, _ := entry["value"].(string); maskedValue == "supersecretvalue" || strings.Contains(maskedValue, "supersecretvalue") {
		t.Fatalf("expected token value to be masked, got %s", maskedValue)
	}

	providerEntry, ok := masked["telegram"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider fallback key to be present")
	}
	if maskedValue, _ := providerEntry["value"].(string); maskedValue == "abcd1234" {
		t.Fatalf("expected provider entry to be masked")
	}
}

func TestMaskValuesEmptyInput(t *testing.T) {
	if out := MaskValues(nil); out != nil {
		t.Fatalf("expected nil output for nil input, got %v", out)
	}
	if out := MaskValues(map[Reference]SecretValue{}); out != nil {
		t.Fatalf("expected nil output for empty input, got %v", out)
	}
}
