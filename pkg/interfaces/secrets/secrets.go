package secrets

import "context"

// Record represents an encrypted secret entry persisted by a store.
type Record struct {
	Scope     string
	SubjectID string
	Channel   string
	Provider  string
	Key       string
	Version   string
	Cipher    []byte
	Nonce     []byte
	Metadata  map[string]any
	CreatedAt any
	UpdatedAt any
	DeletedAt any
}

// Store defines persistence operations for secret records.
type Store interface {
	Put(ctx context.Context, rec Record) error
	GetLatest(ctx context.Context, scope, subjectID, channel, provider, key string) (Record, error)
	GetVersion(ctx context.Context, scope, subjectID, channel, provider, key, version string) (Record, error)
	Delete(ctx context.Context, scope, subjectID, channel, provider, key string) error
	List(ctx context.Context, scope, subjectID, channel, provider, key string) ([]Record, error)
}
