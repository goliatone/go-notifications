# Inbox Guide

This guide covers how to implement in-app notification centers with real-time updates using the inbox system in `go-notifications`.

## Table of Contents

1. [Overview](#overview)
2. [Inbox Architecture](#inbox-architecture)
3. [InboxItem Entity](#inboxitem-entity)
4. [Creating Inbox Items](#creating-inbox-items)
5. [Listing and Filtering](#listing-and-filtering)
6. [Read/Unread Tracking](#readunread-tracking)
7. [Snooze and Dismiss](#snooze-and-dismiss)
8. [Badge Counts](#badge-counts)
9. [Real-Time Broadcasting](#real-time-broadcasting)
10. [Building Notification Center UIs](#building-notification-center-uis)
11. [Troubleshooting](#troubleshooting)

---

## Overview

The inbox system provides in-app notification storage and real-time delivery. Key features:

- **Persistent storage**: Store notifications for later viewing
- **Read/unread tracking**: Badge counts and unread state management
- **Snooze**: Defer notifications to resurface later
- **Dismiss**: Remove notifications from the active queue
- **Pinned items**: Keep important notifications at the top
- **Real-time delivery**: Push updates via WebSocket, SSE, or webhooks
- **Activity hooks**: Track user interactions for analytics

---

## Inbox Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Notification Event                        │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                       Dispatcher                             │
│  (renders templates, routes to adapters)                     │
└─────────────────────────┬───────────────────────────────────┘
                          │
          ┌───────────────┼───────────────┐
          ▼               ▼               ▼
    ┌──────────┐   ┌──────────┐   ┌──────────┐
    │  Email   │   │   SMS    │   │  Inbox   │
    │ Adapter  │   │ Adapter  │   │ Service  │
    └──────────┘   └──────────┘   └────┬─────┘
                                       │
                          ┌────────────┼────────────┐
                          ▼            ▼            ▼
                   ┌──────────┐  ┌──────────┐  ┌──────────┐
                   │ Database │  │Broadcaster│  │ Activity │
                   │ Storage  │  │(WebSocket)│  │  Hooks   │
                   └──────────┘  └──────────┘  └──────────┘
```

---

## InboxItem Entity

```go
type InboxItem struct {
    ID           uuid.UUID   // Unique identifier
    UserID       string      // Owner of the inbox item
    MessageID    uuid.UUID   // Link to NotificationMessage
    Title        string      // Notification title
    Body         string      // Notification body
    Locale       string      // Content locale
    Unread       bool        // Read/unread state
    Pinned       bool        // Pinned to top
    ActionURL    string      // Click-through URL
    Metadata     JSONMap     // Custom metadata
    ReadAt       time.Time   // When marked as read
    DismissedAt  time.Time   // When dismissed
    SnoozedUntil time.Time   // Snooze until timestamp
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

---

## Creating Inbox Items

### Initialize the Service

```go
import (
    "github.com/goliatone/go-notifications/pkg/inbox"
    "github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

inboxService, err := inbox.New(inbox.Dependencies{
    Repository:  inboxRepo,
    Broadcaster: wsBroadcaster,  // WebSocket broadcaster
    Logger:      logger,
    Activity:    activityHooks,  // Optional activity tracking
})
```

### Create an Inbox Item Manually

```go
item, err := inboxService.Create(ctx, inbox.CreateInput{
    UserID:    "user-123",
    MessageID: messageUUID,  // Link to original message
    Title:     "Order Shipped",
    Body:      "Your order #12345 has shipped and is on the way!",
    Locale:    "en",
    ActionURL: "/orders/12345",
    Pinned:    false,
    Metadata: domain.JSONMap{
        "order_id": "12345",
        "carrier":  "UPS",
    },
})
```

### Automatic Delivery from Messages

The dispatcher automatically creates inbox items when the channel is `inbox`:

```go
// Definition with inbox channel
definition := &domain.NotificationDefinition{
    Code:         "order-shipped",
    Channels:     domain.StringList{"email:sendgrid", "inbox"},
    TemplateKeys: domain.StringList{"email:order-shipped", "inbox:order-shipped"},
}
```

Or use `DeliverFromMessage` to convert a rendered message:

```go
msg := &domain.NotificationMessage{
    ID:       messageUUID,
    Subject:  "Order Shipped",
    Body:     "Your order has shipped!",
    Receiver: "user-123",
    Locale:   "en",
    ActionURL: "/orders/12345",
}

err := inboxService.DeliverFromMessage(ctx, msg)
```

---

## Listing and Filtering

### Basic Listing

```go
result, err := inboxService.List(ctx, "user-123", store.ListOptions{
    Limit:  20,
    Offset: 0,
}, inbox.ListFilters{})

for _, item := range result.Items {
    fmt.Printf("[%s] %s: %s\n",
        boolToRead(item.Unread),
        item.Title,
        item.CreatedAt.Format(time.RFC3339),
    )
}
```

### Filter Options

```go
type ListFilters struct {
    UnreadOnly       bool       // Only unread items
    IncludeDismissed bool       // Include dismissed items
    PinnedOnly       bool       // Only pinned items
    SnoozedOnly      bool       // Only snoozed items
    Before           time.Time  // Items created before timestamp
}
```

### Examples

**Unread only**:
```go
result, _ := inboxService.List(ctx, userID, opts, inbox.ListFilters{
    UnreadOnly: true,
})
```

**Pinned notifications**:
```go
result, _ := inboxService.List(ctx, userID, opts, inbox.ListFilters{
    PinnedOnly: true,
})
```

**Exclude dismissed**:
```go
result, _ := inboxService.List(ctx, userID, opts, inbox.ListFilters{
    IncludeDismissed: false,  // Default behavior
})
```

**Include dismissed (archive view)**:
```go
result, _ := inboxService.List(ctx, userID, opts, inbox.ListFilters{
    IncludeDismissed: true,
})
```

---

## Read/Unread Tracking

### Mark as Read

```go
// Mark single item as read
err := inboxService.MarkRead(ctx, "user-123", []string{itemID}, true)

// Mark multiple items as read
err := inboxService.MarkRead(ctx, "user-123", []string{
    "550e8400-e29b-41d4-a716-446655440000",
    "550e8400-e29b-41d4-a716-446655440001",
}, true)
```

### Mark as Unread

```go
err := inboxService.MarkRead(ctx, "user-123", []string{itemID}, false)
```

### Security Note

The service ignores IDs that don't belong to the requesting user, preventing enumeration attacks:

```go
// This will silently skip items not owned by user-123
err := inboxService.MarkRead(ctx, "user-123", []string{
    "other-users-item-id",  // Skipped
    "my-item-id",           // Processed
}, true)
```

---

## Snooze and Dismiss

### Snooze an Item

Defer a notification to resurface later:

```go
// Snooze until tomorrow at 9 AM
until := time.Now().Add(24 * time.Hour).Truncate(time.Hour).Add(9 * time.Hour)
unixTimestamp := until.Unix()

err := inboxService.Snooze(ctx, "user-123", itemID, unixTimestamp)
```

### Dismiss an Item

Remove from the active inbox (soft delete):

```go
err := inboxService.Dismiss(ctx, "user-123", itemID)
```

Dismissed items:
- Have `Unread` set to `false`
- Have `DismissedAt` set to current time
- Are excluded from listings by default (unless `IncludeDismissed: true`)

---

## Badge Counts

### Get Unread Count

```go
count, err := inboxService.BadgeCount(ctx, "user-123")
fmt.Printf("You have %d unread notifications\n", count)
```

### Use in API Responses

```go
type InboxResponse struct {
    Items       []InboxItemView `json:"items"`
    UnreadCount int             `json:"unread_count"`
    Total       int             `json:"total"`
}

func getInbox(userID string) (*InboxResponse, error) {
    result, _ := inboxService.List(ctx, userID, opts, filters)
    count, _ := inboxService.BadgeCount(ctx, userID)

    return &InboxResponse{
        Items:       toViews(result.Items),
        UnreadCount: count,
        Total:       result.Total,
    }, nil
}
```

---

## Real-Time Broadcasting

### Broadcaster Interface

```go
type Broadcaster interface {
    Broadcast(ctx context.Context, event Event) error
}

type Event struct {
    Topic   string  // Event type (e.g., "inbox.created")
    Payload any     // Event data
}
```

### Event Topics

| Topic | Trigger |
|-------|---------|
| `inbox.created` | New inbox item created |
| `inbox.updated` | Item marked read/unread, snoozed, or dismissed |

### Event Payload

```go
{
    "id":         "550e8400-e29b-41d4-a716-446655440000",
    "user_id":    "user-123",
    "title":      "Order Shipped",
    "unread":     true,
    "dismissed":  false,
    "snoozed_at": "0001-01-01T00:00:00Z"
}
```

### WebSocket Implementation Example

```go
type WebSocketBroadcaster struct {
    hub *WebSocketHub
}

func (b *WebSocketBroadcaster) Broadcast(ctx context.Context, event broadcaster.Event) error {
    // Extract user ID from payload
    payload, ok := event.Payload.(map[string]any)
    if !ok {
        return nil
    }
    userID, _ := payload["user_id"].(string)
    if userID == "" {
        return nil
    }

    // Send to user's WebSocket connections
    message, _ := json.Marshal(map[string]any{
        "type":    event.Topic,
        "payload": payload,
    })
    return b.hub.SendToUser(userID, message)
}
```

### SSE Implementation Example

```go
type SSEBroadcaster struct {
    clients map[string]chan []byte
    mu      sync.RWMutex
}

func (b *SSEBroadcaster) Broadcast(ctx context.Context, event broadcaster.Event) error {
    payload, ok := event.Payload.(map[string]any)
    if !ok {
        return nil
    }
    userID, _ := payload["user_id"].(string)

    b.mu.RLock()
    ch, exists := b.clients[userID]
    b.mu.RUnlock()

    if !exists {
        return nil
    }

    data, _ := json.Marshal(map[string]any{
        "event": event.Topic,
        "data":  payload,
    })

    select {
    case ch <- data:
    default:
        // Channel full, drop message
    }
    return nil
}
```

### Webhook Implementation Example

```go
type WebhookBroadcaster struct {
    client  *http.Client
    baseURL string
}

func (b *WebhookBroadcaster) Broadcast(ctx context.Context, event broadcaster.Event) error {
    payload, _ := json.Marshal(map[string]any{
        "topic":   event.Topic,
        "payload": event.Payload,
    })

    req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/webhooks/inbox", bytes.NewReader(payload))
    req.Header.Set("Content-Type", "application/json")

    resp, err := b.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

---

## Building Notification Center UIs

### REST API Endpoints

```go
// GET /api/inbox
func listInbox(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)
    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
    unreadOnly := r.URL.Query().Get("unread") == "true"

    result, err := inboxService.List(ctx, userID,
        store.ListOptions{Limit: limit, Offset: offset},
        inbox.ListFilters{UnreadOnly: unreadOnly},
    )
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    count, _ := inboxService.BadgeCount(ctx, userID)

    json.NewEncoder(w).Encode(map[string]any{
        "items":        result.Items,
        "total":        result.Total,
        "unread_count": count,
    })
}

// POST /api/inbox/mark-read
func markRead(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)

    var body struct {
        IDs  []string `json:"ids"`
        Read bool     `json:"read"`
    }
    json.NewDecoder(r.Body).Decode(&body)

    err := inboxService.MarkRead(ctx, userID, body.IDs, body.Read)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    w.WriteHeader(http.StatusOK)
}

// POST /api/inbox/:id/snooze
func snoozeItem(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)
    itemID := chi.URLParam(r, "id")

    var body struct {
        Until int64 `json:"until"`  // Unix timestamp
    }
    json.NewDecoder(r.Body).Decode(&body)

    err := inboxService.Snooze(ctx, userID, itemID, body.Until)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    w.WriteHeader(http.StatusOK)
}

// POST /api/inbox/:id/dismiss
func dismissItem(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)
    itemID := chi.URLParam(r, "id")

    err := inboxService.Dismiss(ctx, userID, itemID)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    w.WriteHeader(http.StatusOK)
}

// GET /api/inbox/badge
func getBadge(w http.ResponseWriter, r *http.Request) {
    userID := getUserID(r)
    count, _ := inboxService.BadgeCount(ctx, userID)

    json.NewEncoder(w).Encode(map[string]int{"count": count})
}
```

### Frontend Integration (React Example)

```typescript
// Hook for real-time inbox updates
function useInbox() {
    const [items, setItems] = useState<InboxItem[]>([]);
    const [unreadCount, setUnreadCount] = useState(0);

    useEffect(() => {
        // Initial fetch
        fetchInbox();

        // WebSocket for real-time updates
        const ws = new WebSocket('/ws/inbox');
        ws.onmessage = (event) => {
            const { type, payload } = JSON.parse(event.data);

            if (type === 'inbox.created') {
                setItems(prev => [payload, ...prev]);
                setUnreadCount(prev => prev + 1);
            } else if (type === 'inbox.updated') {
                setItems(prev => prev.map(item =>
                    item.id === payload.id ? { ...item, ...payload } : item
                ));
                // Refresh badge count
                fetchBadgeCount();
            }
        };

        return () => ws.close();
    }, []);

    const markRead = async (ids: string[], read: boolean) => {
        await fetch('/api/inbox/mark-read', {
            method: 'POST',
            body: JSON.stringify({ ids, read }),
        });
    };

    const dismiss = async (id: string) => {
        await fetch(`/api/inbox/${id}/dismiss`, { method: 'POST' });
        setItems(prev => prev.filter(item => item.id !== id));
    };

    return { items, unreadCount, markRead, dismiss };
}
```

---

## Troubleshooting

### "inbox: user_id is required"

The `UserID` field is mandatory:

```go
inbox.CreateInput{
    UserID: "user-123",  // Required
    Title:  "...",
    Body:   "...",
}
```

### "inbox: title is required"

Both `Title` and `Body` are required:

```go
inbox.CreateInput{
    UserID: "user-123",
    Title:  "Order Shipped",  // Required
    Body:   "Your order...",  // Required
}
```

### Items Not Appearing in List

1. Check if items are dismissed (use `IncludeDismissed: true` to see them)
2. Check if filtering by `UnreadOnly` and items are read
3. Verify the `UserID` matches

### Real-Time Updates Not Working

1. Verify broadcaster is configured:
```go
inbox.New(inbox.Dependencies{
    Broadcaster: myBroadcaster,  // Not nil
})
```

2. Check broadcaster implementation handles the event

3. Verify WebSocket/SSE connection is established

### Badge Count Incorrect

Badge count only includes:
- Items where `Unread = true`
- Items not dismissed (`DismissedAt` is zero)

```go
// Force refresh after operations
count, _ := inboxService.BadgeCount(ctx, userID)
```

---

## Quick Reference

### Service Methods

```go
// Create inbox item
inboxService.Create(ctx, input)

// List with filters
inboxService.List(ctx, userID, listOptions, listFilters)

// Mark as read/unread
inboxService.MarkRead(ctx, userID, ids, read)

// Snooze until timestamp
inboxService.Snooze(ctx, userID, id, unixTimestamp)

// Dismiss item
inboxService.Dismiss(ctx, userID, id)

// Get unread count
inboxService.BadgeCount(ctx, userID)

// Create from rendered message
inboxService.DeliverFromMessage(ctx, message)
```

### List Filters

```go
inbox.ListFilters{
    UnreadOnly:       true,   // Only unread
    IncludeDismissed: false,  // Exclude dismissed
    PinnedOnly:       false,  // Only pinned
    SnoozedOnly:      false,  // Only snoozed
    Before:           time.Time{}, // Created before
}
```

### Broadcast Events

| Event | Payload Fields |
|-------|---------------|
| `inbox.created` | id, user_id, title, unread, dismissed, snoozed_at |
| `inbox.updated` | id, user_id, title, unread, dismissed, snoozed_at |
