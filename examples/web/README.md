# Notification Center Web Example

This example demonstrates the full functionality of the `go-notifications` module through an interactive web application.

## Features

- **Multi-Channel Notifications**: Email, SMS (simulated), and in-app delivery
- **Real-Time Updates**: WebSocket-based live notifications
- **Inbox Management**: Read/unread tracking, dismiss, snooze
- **User Preferences**: Per-definition and per-channel opt-in/out
- **Localization**: English and Spanish template support
- **Admin Functions**: Broadcast alerts, view delivery stats
- **Test Notifications**: Send test messages to verify configuration

## Quick Start

```bash
cd examples/web
go run .
```

Then open your browser to: [http://localhost:8481](http://localhost:8481)

## Demo Users

The example comes with three pre-configured users:

1. **Alice** (alice@example.com) - English locale, Admin access
2. **Bob** (bob@example.com) - English locale, Regular user
3. **Carlos** (carlos@example.com) - Spanish locale, Regular user

Simply click on a user to login (no password required for demo).

## Application Structure

```
examples/web/
├── main.go              # Application entry point
├── app.go               # App initialization and DI
├── config/
│   └── config.go        # Configuration structs
├── handlers.go          # HTTP request handlers
├── middleware.go        # Auth and context middleware
├── routes.go            # Route registration
├── websocket.go         # WebSocket broadcaster
├── seed.go              # Demo data seeding
├── public/
│   ├── index.html       # Dashboard UI
│   ├── app.js           # Frontend JavaScript
│   └── styles.css       # Styling
└── data/
    ├── locales/         # i18n message catalogs
    └── templates/       # Notification templates
```

## API Endpoints

### Public
- `POST /auth/login` - Login with email
- `POST /auth/logout` - Logout

### User APIs (Protected)
- `GET /api/inbox` - List inbox items
- `POST /api/inbox/:id/read` - Mark as read
- `POST /api/inbox/:id/dismiss` - Dismiss notification
- `POST /api/inbox/:id/snooze` - Snooze until timestamp
- `GET /api/preferences` - Get notification preferences
- `PUT /api/preferences` - Update preferences
- `POST /api/notify/test` - Send test notification

### Admin APIs
- `POST /admin/broadcast` - Broadcast to all users
- `GET /admin/definitions` - List notification definitions
- `POST /admin/templates` - Create templates
- `GET /admin/stats` - View delivery statistics

### WebSocket
- `GET /ws?user_id=<id>` - Real-time notification stream

## Demo Scenarios

### 1. Welcome Flow (Multi-Channel)
1. Login as Bob
2. Click "Send Test Notification"
3. Observe console logs showing email/SMS delivery
4. See real-time toast notification
5. Check inbox for new in-app message

### 2. Real-Time Broadcast
1. Login as Alice (admin)
2. Click "Broadcast Alert"
3. Enter a message
4. Login as Bob in another browser/tab
5. See both users receive the alert instantly

### 3. Preference Management
1. Go to Preferences sidebar
2. Toggle email notifications off
3. Send test notification
4. Observe only in-app delivery (no email logged)

### 4. Localized Content
1. Login as Carlos (Spanish)
2. Send test notification
3. Observe Spanish templates in inbox and console logs

### 5. Inbox Management
- Click "Mark Read" to update status
- Click "Dismiss" to remove notification
- Filter by "Unread only"
- View unread count badge

## Configuration

Edit `config/config.go` to customize:

```go
config.Config{
    Server: ServerConfig{
        Host: "localhost",
        Port: "8481",
    },
    Features: FeatureFlags{
        EnableWebSocket: true,
        EnableDigests:   false,
        EnableRetries:   true,
    },
}
```

## Technology Stack

- **Backend**: Go 1.24
- **HTTP Framework**: Fiber v2
- **WebSocket**: gofiber/contrib/websocket
- **Database**: SQLite (in-memory)
- **Storage**: Memory providers (for demo)
- **Frontend**: Vanilla HTML/CSS/JavaScript

## Next Steps

For production use:
1. Replace memory storage with Bun-backed repositories
2. Implement proper authentication (OAuth, JWT)
3. Add real email/SMS providers (SendGrid, Twilio)
4. Use PostgreSQL or MySQL instead of SQLite
5. Add rate limiting and security middleware
6. Deploy with proper WebSocket infrastructure

## Troubleshooting

**WebSocket not connecting?**
- Check browser console for errors
- Ensure port 8481 is not blocked
- Verify WebSocket feature is enabled

**Notifications not appearing?**
- Check server console logs
- Verify definitions and templates were seeded
- Ensure user is logged in

**Spanish templates not working?**
- Login as Carlos to test
- Check locale badge shows "ES"
- Verify templates exist for both locales

## Learn More

- [Main Documentation](../../docs/NTF_TDD.md)
- [Example TDD](../../docs/EXAMPLE_TDD.md)
- [CLAUDE.md](../../CLAUDE.md)
