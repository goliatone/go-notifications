package queue

import (
	"context"
	"time"
)

// Job represents a scheduled unit of work (e.g., send notification later).
type Job struct {
	Key     string
	Payload any
	RunAt   time.Time
}

// Queue represents the go-job compatible enqueue interface required by the dispatcher.
type Queue interface {
	Enqueue(ctx context.Context, job Job) error
}

// Nop queue swallows jobs (used for tests or disabled scheduling).
type Nop struct{}

var _ Queue = (*Nop)(nil)

func (n *Nop) Enqueue(ctx context.Context, job Job) error { return nil }
