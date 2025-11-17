package broadcaster

import "context"

// Func adapts a function to the Broadcaster interface.
type Func func(ctx context.Context, event Event) error

// Broadcast satisfies the Broadcaster interface.
func (f Func) Broadcast(ctx context.Context, event Event) error {
	if f == nil {
		return nil
	}
	return f(ctx, event)
}

// Fanout forwards events to multiple downstream broadcasters.
type Fanout struct {
	targets []Broadcaster
}

// NewFanout assembles a broadcaster that multicasts to the provided targets.
func NewFanout(targets ...Broadcaster) *Fanout {
	filtered := make([]Broadcaster, 0, len(targets))
	for _, target := range targets {
		if target != nil {
			filtered = append(filtered, target)
		}
	}
	return &Fanout{targets: filtered}
}

var _ Broadcaster = (*Fanout)(nil)

// Broadcast delivers the event to each target, returning the first error observed.
func (f *Fanout) Broadcast(ctx context.Context, event Event) error {
	var firstErr error
	for _, target := range f.targets {
		if err := target.Broadcast(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
