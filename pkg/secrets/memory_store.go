package secrets

import (
	"context"
	"sync"

	iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
)

// MemoryStore is a simple in-memory implementation of a secret Store.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]iface.Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]iface.Record)}
}

func (m *MemoryStore) Put(_ context.Context, rec iface.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[keyFromRecord(rec)] = rec
	return nil
}

func (m *MemoryStore) GetLatest(_ context.Context, scope, subjectID, channel, provider, key string) (iface.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest iface.Record
	var found bool
	for _, rec := range m.items {
		if rec.Scope == scope && rec.SubjectID == subjectID && rec.Channel == channel && rec.Provider == provider && rec.Key == key {
			if !found || rec.Version > latest.Version {
				latest = rec
				found = true
			}
		}
	}
	if !found {
		return iface.Record{}, ErrNotFound
	}
	return latest, nil
}

func (m *MemoryStore) GetVersion(_ context.Context, scope, subjectID, channel, provider, key, version string) (iface.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rec := range m.items {
		if rec.Scope == scope && rec.SubjectID == subjectID && rec.Channel == channel && rec.Provider == provider && rec.Key == key && rec.Version == version {
			return rec, nil
		}
	}
	return iface.Record{}, ErrNotFound
}

func (m *MemoryStore) Delete(_ context.Context, scope, subjectID, channel, provider, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, rec := range m.items {
		if rec.Scope == scope && rec.SubjectID == subjectID && rec.Channel == channel && rec.Provider == provider && rec.Key == key {
			delete(m.items, k)
		}
	}
	return nil
}

func (m *MemoryStore) List(_ context.Context, scope, subjectID, channel, provider, key string) ([]iface.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []iface.Record
	for _, rec := range m.items {
		if scope != "" && rec.Scope != scope {
			continue
		}
		if subjectID != "" && rec.SubjectID != subjectID {
			continue
		}
		if channel != "" && rec.Channel != channel {
			continue
		}
		if provider != "" && rec.Provider != provider {
			continue
		}
		if key != "" && rec.Key != key {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func keyFromRecord(rec iface.Record) string {
	return rec.Scope + "|" + rec.SubjectID + "|" + rec.Channel + "|" + rec.Provider + "|" + rec.Key + "|" + rec.Version
}
