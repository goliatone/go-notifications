package secrets

import (
	"sync"
	"time"
)

// CachingResolver wraps another resolver and caches successful lookups for a short TTL.
// Intended for external secret managers to avoid excessive network calls.
type CachingResolver struct {
	Resolver Resolver
	TTL      time.Duration

	now   func() time.Time
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	value   SecretValue
	expires time.Time
}

// NewCachingResolver builds a resolver that caches results for the provided TTL.
// If ttl <= 0, the inner resolver is returned unchanged.
func NewCachingResolver(inner Resolver, ttl time.Duration) Resolver {
	if ttl <= 0 {
		return inner
	}
	return &CachingResolver{
		Resolver: inner,
		TTL:      ttl,
		now:      time.Now,
		cache:    make(map[string]cacheEntry),
	}
}

// Resolve returns cached entries when fresh and only fetches missing refs from the inner resolver.
// Errors from the inner resolver are propagated without caching failed lookups.
func (c *CachingResolver) Resolve(refs ...Reference) (map[Reference]SecretValue, error) {
	if c == nil || c.Resolver == nil {
		return nil, ErrUnsupported
	}
	if c.TTL <= 0 {
		return c.Resolver.Resolve(refs...)
	}
	now := c.now()

	results := make(map[Reference]SecretValue, len(refs))
	missing := make([]Reference, 0, len(refs))

	c.mu.Lock()
	for _, ref := range refs {
		if entry, ok := c.cache[key(ref)]; ok && (entry.expires.IsZero() || entry.expires.After(now)) {
			results[ref] = entry.value
			continue
		}
		missing = append(missing, ref)
	}
	c.mu.Unlock()

	if len(missing) > 0 {
		fresh, err := c.Resolver.Resolve(missing...)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		for ref, val := range fresh {
			c.cache[key(ref)] = cacheEntry{
				value:   val,
				expires: now.Add(c.TTL),
			}
			results[ref] = val
		}
		c.mu.Unlock()
	}

	return results, nil
}
