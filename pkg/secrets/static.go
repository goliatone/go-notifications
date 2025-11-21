package secrets

import (
	"sync"
	"time"
)

// StaticProvider keeps secrets in memory (no encryption). Intended for tests/demo.
type StaticProvider struct {
	mu    sync.RWMutex
	store map[string]SecretValue
}

// NewStaticProvider builds an in-memory provider seeded with optional values.
func NewStaticProvider(seed map[Reference]SecretValue) *StaticProvider {
	p := &StaticProvider{store: make(map[string]SecretValue)}
	for ref, val := range seed {
		p.store[key(ref)] = val
	}
	return p
}

func (p *StaticProvider) Get(ref Reference) (SecretValue, error) {
	if err := ValidateReference(ref); err != nil {
		return SecretValue{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if ref.Version != "" {
		if val, ok := p.store[key(ref)]; ok {
			return val, nil
		}
		return SecretValue{}, ErrNotFound
	}
	// If no version requested, return latest by lexical max.
	var latest SecretValue
	var found bool
	for k, v := range p.store {
		if matchesBase(ref, k) {
			if !found || v.Version > latest.Version {
				latest = v
				found = true
			}
		}
	}
	if !found {
		return SecretValue{}, ErrNotFound
	}
	return latest, nil
}

func (p *StaticProvider) Put(ref Reference, value []byte) (string, error) {
	if err := ValidateReference(ref); err != nil {
		return "", err
	}
	if len(value) == 0 {
		return "", ErrEmptyValue
	}
	if ref.Version == "" {
		ref.Version = time.Now().UTC().Format(time.RFC3339Nano)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.store[key(ref)] = SecretValue{
		Data:      append([]byte(nil), value...),
		Version:   ref.Version,
		Retrieved: time.Now().UTC(),
	}
	return ref.Version, nil
}

func (p *StaticProvider) Delete(ref Reference) error {
	if err := ValidateReference(ref); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.store, key(ref))
	return nil
}

func (p *StaticProvider) Describe(ref Reference) (map[string]any, error) {
	if err := ValidateReference(ref); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if ref.Version != "" {
		if _, ok := p.store[key(ref)]; !ok {
			return nil, ErrNotFound
		}
		return map[string]any{"version": ref.Version}, nil
	}
	latest, err := p.Get(ref)
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": latest.Version}, nil
}

func key(ref Reference) string {
	return string(ref.Scope) + "|" + ref.SubjectID + "|" + ref.Channel + "|" + ref.Provider + "|" + ref.Key + "|" + ref.Version
}

func matchesBase(ref Reference, key string) bool {
	prefix := string(ref.Scope) + "|" + ref.SubjectID + "|" + ref.Channel + "|" + ref.Provider + "|" + ref.Key + "|"
	return len(key) > len(prefix) && key[:len(prefix)] == prefix
}
