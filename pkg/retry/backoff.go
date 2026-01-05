package retry

import "time"

// Backoff computes the delay before the next retry attempt.
type Backoff interface {
	Next(attempt int) time.Duration
}

// ExponentialBackoff grows delays by powers of two, capped at Max.
type ExponentialBackoff struct {
	Base time.Duration
	Max  time.Duration
}

// Next returns the delay for the given attempt (1-based).
func (b ExponentialBackoff) Next(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := b.Base
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	delay := base << (attempt - 1)
	if b.Max > 0 && delay > b.Max {
		return b.Max
	}
	return delay
}

// DefaultBackoff returns the default exponential retry policy.
func DefaultBackoff() Backoff {
	return ExponentialBackoff{
		Base: 100 * time.Millisecond,
		Max:  5 * time.Second,
	}
}
