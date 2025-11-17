package broadcaster

import "context"

// Event carries in-app notification payloads destined for real-time transports.
type Event struct {
	Topic   string
	Payload any
}

// Broadcaster pushes events to WebSocket/SSE/webhook transports.
type Broadcaster interface {
	Broadcast(ctx context.Context, event Event) error
}

// Nop broadcaster discards events.
type Nop struct{}

var _ Broadcaster = (*Nop)(nil)

func (n *Nop) Broadcast(ctx context.Context, event Event) error { return nil }
