# go-notifications Web Example - Technical Design Document

## Overview

This document specifies the implementation of a comprehensive web-based example application that showcases all major features of the `go-notifications` module. The example follows patterns established in sibling packages (`go-cms`, `go-auth`, `go-users`) and demonstrates real-world notification system usage.

## Example Concept

**Multi-Tenant Notification Center** - A web application with:
- Interactive notification dashboard
- Multi-channel delivery (email/SMS/in-app)
- Real-time WebSocket updates
- User preference management
- Inbox with read/unread/snooze/dismiss
- Template and definition management
- Delivery statistics and monitoring

## Architecture

### Technology Stack

- **HTTP Server**: `go-router` with Fiber (consistent with go-users/go-auth)
- **Database**: SQLite in-memory (fast demo, no external deps)
- **WebSocket**: Custom broadcaster implementation
- **Authentication**: Simple session-based auth (not the focus)
- **UI**: Embedded HTML/CSS/JS with vanilla JavaScript
- **Assets**: `//go:embed` for static files

### Directory Structure

```
examples/
  web/
    main.go              # Application entry point, DI setup
    app.go               # App struct and configuration
    routes.go            # HTTP route registration
    handlers.go          # HTTP handlers for API endpoints
    websocket.go         # WebSocket broadcaster implementation
    seed.go              # Bootstrap definitions, templates, sample data
    middleware.go        # Auth and context middleware
    config/
      config.go          # Configuration structs (cfgx pattern)
    public/
      index.html         # Main dashboard UI
      app.js             # Client-side WebSocket + API interactions
      styles.css         # Basic styling
    data/
      locales/           # i18n message catalogs (en.json, es.json)
      templates/         # Sample notification templates
```

## Core Components

### 1. Application Structure (`app.go`)

```go
type App struct {
    config    *config.Config
    module    *notifier.Module
    bunDB     *bun.DB
    router    router.Server[*fiber.App]
    logger    logger.Logger
    wsHub     *WebSocketHub

    // Demo users for multi-user testing
    users     map[string]*DemoUser
}

type DemoUser struct {
    ID       string
    Name     string
    Email    string
    Locale   string
    TenantID string
}
```

### 2. WebSocket Broadcaster (`websocket.go`)

Implements `pkg/interfaces/broadcaster.Broadcaster` for real-time inbox updates.

```go
type WebSocketHub struct {
    clients    map[string]*WebSocketClient  // userID -> client
    register   chan *WebSocketClient
    unregister chan *WebSocketClient
    broadcast  chan BroadcastMessage
    logger     logger.Logger
}

type BroadcastMessage struct {
    UserID  string
    Event   string
    Payload map[string]any
}

type WebSocketClient struct {
    ID     string
    UserID string
    Conn   *websocket.Conn
    Send   chan []byte
}

// Implements broadcaster.Broadcaster
func (h *WebSocketHub) Broadcast(ctx context.Context, event broadcaster.Event) error
```

### 3. HTTP Routes (`routes.go`)

```go
// Public routes
GET  /                          # Dashboard UI (serves index.html)
GET  /public/*                  # Static assets
POST /auth/login                # Simple login (demo users)
POST /auth/logout               # Logout

// Protected routes (require auth)
GET  /api/inbox                 # List inbox items (paginated)
POST /api/inbox/:id/read        # Mark item as read
POST /api/inbox/:id/unread      # Mark item as unread
POST /api/inbox/:id/dismiss     # Dismiss notification
POST /api/inbox/:id/snooze      # Snooze until timestamp
GET  /api/inbox/stats           # Unread count, etc.

GET  /api/preferences           # Get user preferences
PUT  /api/preferences           # Update preferences

POST /api/notify/test           # Trigger test notification
POST /api/notify/event          # Enqueue custom event

GET  /ws                        # WebSocket connection

// Admin routes
POST /admin/definitions         # Create notification definition
GET  /admin/definitions         # List definitions
POST /admin/templates           # Create/update template
GET  /admin/templates           # List templates
GET  /admin/stats               # Delivery statistics
POST /admin/broadcast           # Admin broadcast to all users
```

### 4. API Handlers (`handlers.go`)

#### Inbox Handlers
```go
func (a *App) ListInbox(ctx router.Context) error
    // Query parameters: page, limit, unread_only
    // Returns: paginated inbox items with metadata

func (a *App) MarkRead(ctx router.Context) error
    // Uses commands.InboxMarkRead via module.Commands()

func (a *App) DismissNotification(ctx router.Context) error
    // Uses commands.InboxDismiss

func (a *App) SnoozeNotification(ctx router.Context) error
    // Uses commands.InboxSnooze

func (a *App) InboxStats(ctx router.Context) error
    // Returns: { unread: 5, total: 42, last_read_at: "..." }
```

#### Preference Handlers
```go
func (a *App) GetPreferences(ctx router.Context) error
    // Returns user's notification preferences

func (a *App) UpdatePreferences(ctx router.Context) error
    // Uses commands.UpsertPreference
    // Payload: { definition_code, channel, enabled, quiet_hours }
```

#### Notification Handlers
```go
func (a *App) SendTestNotification(ctx router.Context) error
    // Sends "test_notification" to current user
    // Demonstrates multi-channel delivery

func (a *App) EnqueueEvent(ctx router.Context) error
    // Uses commands.EnqueueEvent
    // Payload: { definition_code, recipients, context }
```

#### Admin Handlers
```go
func (a *App) CreateDefinition(ctx router.Context) error
    // Uses commands.CreateDefinition

func (a *App) ListDefinitions(ctx router.Context) error
    // Lists all notification definitions

func (a *App) CreateTemplate(ctx router.Context) error
    // Uses commands.SaveTemplate

func (a *App) ListTemplates(ctx router.Context) error
    // Lists all templates

func (a *App) BroadcastNotification(ctx router.Context) error
    // Admin sends notification to all users

func (a *App) DeliveryStats(ctx router.Context) error
    // Returns: delivery attempts, success/failure counts
```

### 5. Seed Data (`seed.go`)

```go
func SeedDemoData(ctx context.Context, app *App) error

// Creates demo users
var DemoUsers = []DemoUser{
    {ID: "user1", Name: "Alice", Email: "alice@example.com", Locale: "en"},
    {ID: "user2", Name: "Bob", Email: "bob@example.com", Locale: "en"},
    {ID: "user3", Name: "Carlos", Email: "carlos@example.com", Locale: "es"},
}

// Creates notification definitions
func seedDefinitions(ctx context.Context, catalog *commands.Catalog) error
    // Definitions:
    // - welcome: Multi-channel welcome message
    // - comment_reply: In-app only, digest eligible
    // - system_alert: High priority, all channels
    // - weekly_digest: Email only, scheduled
    // - test_notification: For testing multi-channel delivery

// Creates templates
func seedTemplates(ctx context.Context, catalog *commands.Catalog) error
    // Templates for each definition x channel x locale
    // welcome.email.en, welcome.email.es
    // welcome.in-app.en, welcome.in-app.es
    // system_alert.email.en, system_alert.sms.en
    // etc.

// Creates sample inbox items
func seedInboxItems(ctx context.Context, inbox *inbox.Service) error
    // Pre-populate some notifications for demo users
```

### 6. Configuration (`config/config.go`)

```go
type Config struct {
    Server      ServerConfig
    Auth        AuthConfig
    Persistence PersistenceConfig
    Locales     []string
    Features    FeatureFlags
}

type ServerConfig struct {
    Host string
    Port string
}

type AuthConfig struct {
    SessionKey     string
    SessionTimeout time.Duration
}

type PersistenceConfig struct {
    Driver  string  // "sqlite" or "memory"
    DSN     string  // ":memory:" or file path
}

type FeatureFlags struct {
    EnableWebSocket bool
    EnableDigests   bool
    EnableRetries   bool
}
```

## Demo Scenarios

### Scenario 1: Welcome Flow (Multi-channel)

**Trigger**: User logs in for the first time

**Flow**:
1. Frontend calls `POST /api/notify/event` with definition_code="welcome"
2. Backend enqueues event via `commands.EnqueueEvent`
3. Dispatcher expands to channels: [email, in-app, sms]
4. Console adapter logs email/SMS (simulated)
5. Inbox service creates in-app item
6. WebSocket broadcasts to user's connected client
7. UI shows toast notification + increments unread badge

**Expected Output**:
- Console logs show rendered email/SMS templates
- Inbox has new unread item
- Real-time update appears instantly

### Scenario 2: System Alert (Real-time Broadcast)

**Trigger**: Admin sends broadcast via `POST /admin/broadcast`

**Flow**:
1. Admin payload: `{ definition_code: "system_alert", message: "Maintenance at 2am" }`
2. Backend enqueues event for all users
3. Dispatcher delivers to all channels
4. WebSocket broadcasts to all connected clients
5. All UIs show alert banner

**Expected Output**:
- All users see notification simultaneously
- Multiple delivery attempts logged
- Unread counts updated

### Scenario 3: Digest Notifications (Batching)

**Trigger**: Multiple comment replies within 1 hour

**Flow**:
1. User receives 5 "comment_reply" events
2. Definition has policy: `{ digest: { window: "1h", max_items: 10 } }`
3. Dispatcher batches events into single digest
4. Single email sent with all 5 comments
5. Single in-app item with "5 new replies"

**Expected Output**:
- 5 events → 1 delivery
- Template shows list of comments
- Demonstrates batching logic

### Scenario 4: Preference Management

**Trigger**: User toggles preferences via UI

**Flow**:
1. UI calls `PUT /api/preferences` with `{ definition_code: "comment_reply", channel: "email", enabled: false }`
2. Backend uses `commands.UpsertPreference`
3. Next "comment_reply" event skips email channel
4. In-app notification still delivered

**Expected Output**:
- Preference saved and effective immediately
- Delivery respects opt-out
- UI reflects current state

### Scenario 5: Localized Templates

**Trigger**: Spanish user receives notification

**Flow**:
1. Carlos (locale="es") logs in
2. Welcome notification triggered
3. Dispatcher resolves locale: "es"
4. Template service loads "welcome.email.es"
5. Rendered with Spanish translations

**Expected Output**:
- Spanish email template rendered
- go-i18n handles fallback if translation missing
- Demonstrates i18n integration

### Scenario 6: Retry & Failure Handling

**Trigger**: Simulated adapter failure

**Flow**:
1. Configure test adapter to fail first 2 attempts
2. Send notification via test adapter
3. Dispatcher retries with exponential backoff
4. Third attempt succeeds
5. Delivery attempt history shows retries

**Expected Output**:
- Multiple delivery attempts logged
- Status transitions: pending → failed → succeeded
- Demonstrates retry logic

## UI Implementation

### Dashboard (`public/index.html`)

```html
<!DOCTYPE html>
<html>
<head>
    <title>Notification Center</title>
    <link rel="stylesheet" href="/public/styles.css">
</head>
<body>
    <div id="app">
        <!-- Header -->
        <nav>
            <h1>Notification Center</h1>
            <div class="user-info">
                <span id="username"></span>
                <span class="badge" id="unread-count">0</span>
                <button id="logout">Logout</button>
            </div>
        </nav>

        <!-- Main Content -->
        <div class="container">
            <!-- Sidebar: Preferences -->
            <aside class="sidebar">
                <h2>Preferences</h2>
                <div id="preferences-list"></div>
            </aside>

            <!-- Main: Inbox -->
            <main class="inbox">
                <h2>Inbox</h2>
                <div class="inbox-controls">
                    <button id="mark-all-read">Mark All Read</button>
                    <button id="test-notification">Send Test</button>
                </div>
                <div id="inbox-list"></div>
                <div id="pagination"></div>
            </main>

            <!-- Admin Panel (if admin user) -->
            <aside class="admin-panel" id="admin-panel" style="display:none">
                <h2>Admin</h2>
                <button id="broadcast-alert">Broadcast Alert</button>
                <div id="stats"></div>
            </aside>
        </div>
    </div>

    <script src="/public/app.js"></script>
</body>
</html>
```

### Client JavaScript (`public/app.js`)

```javascript
class NotificationCenter {
    constructor() {
        this.ws = null
        this.unreadCount = 0
    }

    async init() {
        await this.loadUser()
        await this.loadInbox()
        await this.loadPreferences()
        this.connectWebSocket()
        this.bindEvents()
    }

    connectWebSocket() {
        const wsURL = `ws://${window.location.host}/ws`
        this.ws = new WebSocket(wsURL)

        this.ws.onmessage = (event) => {
            const msg = JSON.parse(event.data)
            this.handleRealtimeEvent(msg)
        }
    }

    handleRealtimeEvent(msg) {
        switch(msg.event) {
            case 'inbox.new':
                this.addInboxItem(msg.payload)
                this.incrementUnread()
                this.showToast('New notification')
                break
            case 'inbox.read':
                this.markItemRead(msg.payload.id)
                this.decrementUnread()
                break
        }
    }

    async loadInbox() {
        const resp = await fetch('/api/inbox?page=1&limit=20')
        const data = await resp.json()
        this.renderInbox(data.items)
        this.unreadCount = data.unread_count
        this.updateBadge()
    }

    async markRead(id) {
        await fetch(`/api/inbox/${id}/read`, { method: 'POST' })
    }

    async dismiss(id) {
        await fetch(`/api/inbox/${id}/dismiss`, { method: 'POST' })
        this.removeItem(id)
    }

    async updatePreference(definitionCode, channel, enabled) {
        await fetch('/api/preferences', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ definition_code: definitionCode, channel, enabled })
        })
    }

    async sendTestNotification() {
        await fetch('/api/notify/test', { method: 'POST' })
    }
}

const app = new NotificationCenter()
app.init()
```

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Create example directory structure
- [ ] Setup `main.go` with DI container
- [ ] Initialize notifier module with memory storage
- [ ] Configure SQLite/memory database
- [ ] Setup go-router with Fiber
- [ ] Implement basic config loading

### Phase 2: Authentication & Middleware
- [ ] Create demo user system
- [ ] Implement simple session auth
- [ ] Auth middleware for protected routes
- [ ] User context middleware

### Phase 3: WebSocket Broadcaster
- [ ] Implement WebSocketHub
- [ ] Implement broadcaster.Broadcaster interface
- [ ] Handle client connections/disconnections
- [ ] Broadcast routing by userID

### Phase 4: API Handlers
- [ ] Inbox handlers (list, read, dismiss, snooze)
- [ ] Preference handlers (get, update)
- [ ] Notification handlers (test, enqueue)
- [ ] Admin handlers (definitions, templates, stats)

### Phase 5: Seed Data
- [ ] Create demo users
- [ ] Seed notification definitions
- [ ] Seed templates (en/es for each channel)
- [ ] Create sample inbox items

### Phase 6: UI Implementation
- [ ] HTML dashboard structure
- [ ] CSS styling
- [ ] WebSocket client connection
- [ ] Inbox rendering and interactions
- [ ] Preference toggles
- [ ] Real-time updates

### Phase 7: Adapters
- [ ] Console adapter (default)
- [ ] Mock SMTP adapter (logs to console)
- [ ] Mock SMS adapter (logs to console)
- [ ] Failing adapter (for retry demo)

### Phase 8: Demo Scenarios
- [ ] Welcome flow implementation
- [ ] System alert broadcast
- [ ] Digest batching (optional)
- [ ] Preference opt-out
- [ ] Localized templates
- [ ] Retry simulation

## Testing Approach

### Manual Testing
```bash
# Start the example
cd examples/web
go run .

# Open browser
open http://localhost:8481

# Login as Alice (en)
# Login as Carlos (es) in incognito

# Test scenarios:
1. Send test notification → verify multi-channel delivery
2. Toggle preferences → verify opt-out respected
3. Admin broadcast → verify real-time to all users
4. Mark read/dismiss → verify state changes
5. Switch locale → verify Spanish templates
```

### Automated Tests (Optional)
```go
// examples/web/handlers_test.go
func TestInboxListHandler(t *testing.T)
func TestPreferenceUpdateHandler(t *testing.T)
func TestWebSocketBroadcast(t *testing.T)
```

## Configuration Example

```yaml
# examples/web/config.yml
server:
  host: localhost
  port: 8481

auth:
  session_key: demo-secret-key
  session_timeout: 1h

persistence:
  driver: sqlite
  dsn: ":memory:"

locales:
  - en
  - es

features:
  enable_websocket: true
  enable_digests: false
  enable_retries: true
```

## Sample Templates

### Welcome Email (English)
```
Subject: Welcome to Notification Center!

Hello {{ .name }},

Welcome to our notification system. You can manage your preferences at any time.

Best regards,
The Team
```

### Welcome Email (Spanish)
```
Subject: ¡Bienvenido al Centro de Notificaciones!

Hola {{ .name }},

Bienvenido a nuestro sistema de notificaciones. Puedes gestionar tus preferencias en cualquier momento.

Saludos,
El Equipo
```

### System Alert (In-App)
```
Title: System Maintenance
Body: {{ .message }}
Action: Learn More
```

## Deployment Notes

The example should be self-contained and runnable with:
```bash
go run examples/web/main.go
```

No external dependencies beyond Go modules. All assets embedded via `//go:embed`.

## Success Metrics

The example successfully demonstrates:
1. ✅ Module initialization with all dependencies
2. ✅ Multi-channel notification delivery
3. ✅ Real-time WebSocket updates
4. ✅ Inbox management (read/unread/dismiss/snooze)
5. ✅ User preference evaluation
6. ✅ Template rendering with i18n
7. ✅ Command pattern usage
8. ✅ Delivery tracking and retries
9. ✅ Admin operations (definitions, templates)
10. ✅ Clean separation of concerns (DI, services, handlers)

## Future Enhancements (Out of Scope)

- OAuth authentication
- PostgreSQL persistence
- Production-ready UI framework (React/Vue)
- Email/SMS provider integration
- Advanced analytics dashboard
- Multi-tenancy demonstration
- Background job processing with go-job
- Digest scheduling with cron
