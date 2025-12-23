package secrets

import (
	"errors"
	"testing"
	"time"
)

type countingResolver struct {
	count int
	data  map[Reference]SecretValue
	err   error
}

func (c *countingResolver) Resolve(refs ...Reference) (map[Reference]SecretValue, error) {
	c.count++
	if c.err != nil {
		return nil, c.err
	}
	out := make(map[Reference]SecretValue, len(refs))
	for _, ref := range refs {
		if val, ok := c.data[ref]; ok {
			out[ref] = val
		} else {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

func TestCachingResolverCachesUntilTTL(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	val := SecretValue{Data: []byte("secret"), Version: "v1"}

	counter := &countingResolver{
		data: map[Reference]SecretValue{ref: val},
	}
	resolver := NewCachingResolver(counter, 50*time.Millisecond).(*CachingResolver)
	resolver.now = func() time.Time { return time.Unix(0, 0) }

	out, err := resolver.Resolve(ref)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := out[ref]; string(got.Data) != "secret" {
		t.Fatalf("unexpected value %s", got.Data)
	}
	if counter.count != 1 {
		t.Fatalf("expected one inner resolve, got %d", counter.count)
	}

	// Second call within TTL should hit cache.
	_, err = resolver.Resolve(ref)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if counter.count != 1 {
		t.Fatalf("expected cached resolve, got %d calls", counter.count)
	}

	// Advance time past TTL and expect a new resolve.
	resolver.now = func() time.Time { return time.Unix(0, int64(100*time.Millisecond)) }
	if _, err := resolver.Resolve(ref); err != nil {
		t.Fatalf("third resolve: %v", err)
	}
	if counter.count != 2 {
		t.Fatalf("expected cache miss after TTL, got %d calls", counter.count)
	}
}

func TestCachingResolverPropagatesErrors(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	counter := &countingResolver{err: errors.New("boom")}
	resolver := NewCachingResolver(counter, time.Minute).(*CachingResolver)
	resolver.now = time.Now

	if _, err := resolver.Resolve(ref); !errors.Is(err, counter.err) {
		t.Fatalf("expected error to propagate")
	}
	if counter.count != 1 {
		t.Fatalf("expected inner resolver to be called once, got %d", counter.count)
	}
}

func TestCachingResolverBypassesWhenTTLDisabled(t *testing.T) {
	ref := Reference{Scope: ScopeUser, SubjectID: "u1", Channel: "chat", Provider: "slack", Key: "token"}
	val := SecretValue{Data: []byte("secret")}
	counter := &countingResolver{data: map[Reference]SecretValue{ref: val}}

	resolver := NewCachingResolver(counter, 0)
	if _, err := resolver.Resolve(ref); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := resolver.Resolve(ref); err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if counter.count != 2 {
		t.Fatalf("expected resolver to be called every time when TTL disabled, got %d", counter.count)
	}
}
