# Realtime Inbox Hooks

Phase 5 introduces first-class inbox mutations paired with broadcaster hooks so go-admin/go-cms transports can mirror state changes over WebSocket or SSE connections.

## Wiring the Broadcaster

```go
import (
    "context"

    "github.com/goliatone/go-notifications/pkg/inbox"
    "github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

type wsHub struct{}

func (h *wsHub) Broadcast(ctx context.Context, evt broadcaster.Event) error {
    // Serialize evt.Payload and push to connected clients subscribed to evt.Topic
    return nil
}

func buildInboxService(repo store.InboxRepository) (*inbox.Service, error) {
    realTime := broadcaster.NewFanout(&wsHub{})
    return inbox.New(inbox.Dependencies{
        Repository:  repo,
        Broadcaster: realTime,
        Logger:      &logger.Nop{},
    })
}
```

Every create/update/dismiss mutation emits topics (`inbox.created`, `inbox.updated`) so transports can update unread counters instantly.

## SSE Handler Example

```go
func (s *Server) serveInboxStream(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    enc := json.NewEncoder(w)
    ch := make(chan broadcaster.Event, 1)
    cancel := s.observeInbox(r.Context(), ch) // host provided hub taps into Broadcast calls
    defer cancel()

    for {
        select {
        case evt := <-ch:
            fmt.Fprint(w, "event: "+evt.Topic+"\n")
            fmt.Fprint(w, "data: ")
            if err := enc.Encode(evt.Payload); err != nil {
                return
            }
            fmt.Fprint(w, "\n")
            w.(http.Flusher).Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

Hosts integrating WebSockets can reuse the same fan-out broadcaster by adapting `Broadcast` to publish on their hub. The dispatcher automatically calls `InboxService.DeliverFromMessage` whenever the `"inbox"` channel is used, so no additional adapter is required—rendered messages become inbox entries and trigger realtime events with badge counts updated via `BadgeCount`.
