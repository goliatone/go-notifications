package secrets

import "time"

// Scope defines the ownership boundary for a secret.
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeTenant Scope = "tenant"
	ScopeUser   Scope = "user"
)

// Reference identifies a specific secret.
type Reference struct {
	Scope     Scope
	SubjectID string
	Channel   string
	Provider  string
	Key       string
	Version   string
}

// SecretValue carries the resolved secret payload.
type SecretValue struct {
	Data      []byte
	Version   string
	Retrieved time.Time
	Metadata  map[string]any
}

// Provider resolves and manages secret values for a given scope.
type Provider interface {
	Get(ref Reference) (SecretValue, error)
	Put(ref Reference, value []byte) (string, error)
	Delete(ref Reference) error
	Describe(ref Reference) (map[string]any, error) // non-sensitive metadata only
}

// Resolver batches resolution of references and returns keyed results.
type Resolver interface {
	Resolve(refs ...Reference) (map[Reference]SecretValue, error)
}
