# Adapters Guide

This guide covers how to configure and use delivery channel adapters in `go-notifications`. Adapters are the bridge between your notifications and external delivery services.

## Table of Contents

1. [Overview](#overview)
2. [Adapter Architecture](#adapter-architecture)
3. [Registry and Routing](#registry-and-routing)
4. [Secure Link Workflow](#secure-link-workflow)
5. [Built-in Adapters](#built-in-adapters)
   - [Console](#console)
   - [SMTP](#smtp)
   - [SendGrid](#sendgrid)
   - [Mailgun](#mailgun)
   - [AWS SES](#aws-ses)
   - [Twilio](#twilio)
   - [WhatsApp](#whatsapp)
   - [Telegram](#telegram)
   - [Slack](#slack)
   - [Firebase](#firebase)
   - [AWS SNS](#aws-sns)
   - [Webhook](#webhook)
6. [Secrets Management](#secrets-management)
7. [Writing Custom Adapters](#writing-custom-adapters)
8. [Multi-Channel Fan-Out](#multi-channel-fan-out)
9. [Retry Policies](#retry-policies)
10. [Troubleshooting](#troubleshooting)

---

## Overview

Adapters implement the `Messenger` interface to deliver notifications through various channels. Each adapter:

- Supports one or more logical channels (email, sms, push, chat, etc.)
- Has a unique provider name for routing
- Handles authentication and API communication
- Reports success/failure for retry handling

---

## Adapter Architecture

### The Messenger Interface

All adapters implement this interface:

```go
type Messenger interface {
    Name() string
    Capabilities() Capability
    Send(ctx context.Context, msg Message) error
}
```

### Capability

Describes what an adapter supports:

```go
type Capability struct {
    Name           string            // Provider identifier (e.g., "sendgrid")
    Channels       []string          // Supported channels (e.g., ["email"])
    Formats        []string          // Content formats (e.g., ["text/plain", "text/html"])
    MaxAttachments int               // Max attachments (0 = unlimited)
    Metadata       map[string]string // Provider-specific metadata
}
```

### Message

The payload passed to `Send()`:

```go
type Message struct {
    ID          string            // Unique message identifier
    Channel     string            // Logical channel (email, sms, etc.)
    Provider    string            // Target provider name
    Subject     string            // Message subject/title
    Body        string            // Primary content
    To          string            // Recipient address
    Attachments []Attachment      // File attachments
    Metadata    map[string]any    // Channel-specific data
    Locale      string            // Content locale
    Headers     map[string]string // Custom headers
    Attempts    int               // Retry count
    TraceID     string            // Distributed tracing ID
    RequestID   string            // Request correlation ID
}
```

---

## Registry and Routing

### Creating a Registry

```go
import (
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/adapters/sendgrid"
    "github.com/goliatone/go-notifications/pkg/adapters/twilio"
)

registry := adapters.NewRegistry(
    console.New(logger),
    sendgrid.New(logger, sendgrid.WithAPIKey("your-api-key")),
    twilio.New(logger, twilio.WithConfig(twilio.Config{
        AccountSID: "your-sid",
        AuthToken:  "your-token",
    })),
)
```

### Channel String Format

Channels use the format `<channel>:<provider>`:

| Channel String | Channel | Provider |
|----------------|---------|----------|
| `email:sendgrid` | email | sendgrid |
| `email:smtp` | email | smtp |
| `sms:twilio` | sms | twilio |
| `push:firebase` | push | firebase |
| `chat:slack` | chat | slack |
| `email` | email | (first registered) |

### Routing Logic

```go
// Explicit provider
adapter, err := registry.Route("email:sendgrid")

// Fallback to first registered for channel
adapter, err := registry.Route("email")

// List all adapters for a channel
adapters := registry.List("email")
```

---

## Secure Link Workflow

Secure links are generated before adapters send messages. Inject a `LinkBuilder` (and optional `LinkStore`/`LinkObserver`) into the module so resolved URLs are available as `action_url`, `manifest_url`, and `url` in templates and message fields.

```go
import (
	linksecure "github.com/goliatone/go-notifications/adapters/securelink"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-urlkit/securelink"
)

manager := securelink.NewManager(cfg)
builder := linksecure.NewBuilder(manager)
store := linksecure.NewMemoryStore()

module, _ := notifier.NewModule(notifier.ModuleOptions{
	LinkBuilder: builder,
	LinkStore:   store,
})
_ = module
```

Templates can access resolved links directly (`action_url`) or via `secure_link(...)` once the helper is registered in your renderer stack.

---

## Built-in Adapters

### Console

Logs notifications to stdout. Ideal for development and debugging.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/console"

adapter := console.New(logger,
    console.WithName("console"),      // Override provider name
    console.WithStructured(true),     // Emit structured logs
)
```

**Channels**: `email`
**Use case**: Development, testing, debugging

---

### SMTP

Delivers email via SMTP with TLS/STARTTLS support.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/smtp"

adapter := smtp.New(logger,
    smtp.WithHostPort("smtp.example.com", 587),
    smtp.WithCredentials("user@example.com", "password"),
    smtp.WithFrom("noreply@example.com"),
    smtp.WithStartTLS(true),
)
```

**Configuration Options**:

| Option | Description |
|--------|-------------|
| `WithHostPort(host, port)` | SMTP server address |
| `WithCredentials(user, pass)` | Authentication |
| `WithFrom(address)` | Default sender |
| `WithTLS(bool)` | Implicit TLS (port 465) |
| `WithStartTLS(bool)` | STARTTLS upgrade (port 587) |
| `WithConfig(Config)` | Full configuration |

**Config struct**:

```go
type Config struct {
    Host          string
    Port          int
    Username      string
    Password      string
    From          string
    UseTLS        bool            // Implicit TLS
    UseStartTLS   bool            // STARTTLS upgrade
    SkipTLSVerify bool            // Skip certificate verification
    Timeout       time.Duration   // Connection timeout
    LocalName     string          // HELO/EHLO hostname
    AuthDisabled  bool            // Skip authentication
    Headers       map[string]string // Default headers
    PlainOnly     bool            // Force text/plain
}
```

**Channels**: `email`

---

### SendGrid

Delivers email via the SendGrid API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/sendgrid"

adapter := sendgrid.New(logger,
    sendgrid.WithAPIKey("SG.xxxxx"),
    sendgrid.WithFrom("noreply@example.com"),
    sendgrid.WithReplyTo("support@example.com"),
)
```

**Configuration Options**:

| Option | Description |
|--------|-------------|
| `WithAPIKey(key)` | SendGrid API key |
| `WithFrom(address)` | Default sender |
| `WithReplyTo(address)` | Reply-to address |
| `WithBaseURL(url)` | API base URL |
| `WithTimeout(seconds)` | Request timeout |
| `WithHTTPClient(client)` | Custom HTTP client |

**Metadata fields**:

| Field | Description |
|-------|-------------|
| `from` | Override sender |
| `reply_to` | Reply-to address |
| `cc` | CC recipients ([]string) |
| `bcc` | BCC recipients ([]string) |
| `text_body` | Plain text content |
| `html_body` | HTML content |

**Channels**: `email`

---

### Mailgun

Delivers email via the Mailgun API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/mailgun"

adapter := mailgun.New(logger,
    mailgun.WithConfig(mailgun.Config{
        Domain: "mg.example.com",
        APIKey: "key-xxxxx",
        From:   "noreply@example.com",
    }),
)
```

**Config struct**:

```go
type Config struct {
    Domain     string   // Mailgun domain
    APIKey     string   // API key
    APIBase    string   // API base URL (default: https://api.mailgun.net/v3)
    From       string   // Default sender
    TimeoutSec int      // Request timeout
}
```

**Metadata fields**: `from`, `reply_to`, `cc`, `bcc`, `text_body`, `html_body`

**Channels**: `email`

---

### AWS SES

Delivers email via Amazon Simple Email Service.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/aws_ses"

adapter := aws_ses.New(logger,
    aws_ses.WithConfig(aws_ses.Config{
        From:             "noreply@example.com",
        Region:           "us-east-1",
        Profile:          "production",
        ConfigurationSet: "my-config-set",
    }),
)
```

**Config struct**:

```go
type Config struct {
    From             string   // Sender address
    Region           string   // AWS region
    Profile          string   // AWS profile name
    ConfigurationSet string   // SES configuration set
    DryRun           bool     // Skip actual delivery
}
```

**Authentication**: Uses AWS SDK default credential chain (env vars, profiles, IAM roles).

**Channels**: `email`

---

### Twilio

Delivers SMS and WhatsApp messages via Twilio's REST API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/twilio"

adapter := twilio.New(logger,
    twilio.WithConfig(twilio.Config{
        AccountSID: "ACxxxxx",
        AuthToken:  "your-token",
        From:       "+15551234567",
    }),
)
```

**Config struct**:

```go
type Config struct {
    AccountSID          string
    AuthToken           string
    From                string        // Default sender phone
    MessagingServiceSID string        // Use messaging service instead of From
    APIBaseURL          string
    Timeout             time.Duration
    SkipTLSVerify       bool
    PlainOnly           bool          // Force plain text
    DryRun              bool          // Skip delivery, log only
}
```

**Metadata fields**:

| Field | Description |
|-------|-------------|
| `from` | Override sender |
| `body` | Message body |
| `media_urls` | MMS media URLs ([]string) |

**WhatsApp**: Automatically prefixes numbers with `whatsapp:` when channel is `whatsapp`.

**Channels**: `sms`, `whatsapp`

---

### WhatsApp

Delivers messages via Meta's WhatsApp Cloud API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/whatsapp"

adapter := whatsapp.New(logger,
    whatsapp.WithConfig(whatsapp.Config{
        Token:         "your-access-token",
        PhoneNumberID: "123456789",
    }),
)
```

**Config struct**:

```go
type Config struct {
    Token         string        // Graph API access token
    PhoneNumberID string        // WhatsApp phone number ID
    APIBase       string        // API base URL
    Timeout       time.Duration
    SkipTLSVerify bool
    PlainOnly     bool
}
```

**Metadata fields**: `body`, `html_body`, `preview_url` (bool)

**Channels**: `whatsapp`, `chat`

---

### Telegram

Delivers messages via the Telegram Bot API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/telegram"

adapter := telegram.New(logger,
    telegram.WithConfig(telegram.Config{
        Token:  "123456:ABC-DEF...",
        ChatID: "-100123456789",
    }),
)
```

**Config struct**:

```go
type Config struct {
    Token                 string        // Bot token
    ChatID                string        // Default chat ID
    BaseURL               string
    ParseMode             string        // HTML, MarkdownV2
    DisableWebPagePreview bool
    DisableNotification   bool
    Timeout               time.Duration
    SkipTLSVerify         bool
    PlainOnly             bool
    DryRun                bool
}
```

**Metadata fields**:

| Field | Description |
|-------|-------------|
| `chat_id` | Target chat ID |
| `parse_mode` | `HTML` or `MarkdownV2` |
| `thread_id` | Forum topic ID |
| `reply_to` | Reply to message ID |
| `disable_preview` | Disable URL preview |
| `silent` | Disable notification sound |

**Channels**: `chat`

---

### Slack

Delivers messages via Slack's `chat.postMessage` API.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/slack"

adapter := slack.New(logger,
    slack.WithConfig(slack.Config{
        Token:   "xoxb-xxxxx",
        Channel: "#notifications",
    }),
)
```

**Config struct**:

```go
type Config struct {
    Token         string
    Channel       string        // Default channel
    BaseURL       string
    Timeout       time.Duration
    SkipTLSVerify bool
    DryRun        bool
}
```

**Metadata fields**:

| Field | Description |
|-------|-------------|
| `channel` | Target channel/user |
| `body` | Message text (mrkdwn) |
| `thread_ts` | Thread timestamp |

**Channels**: `chat`, `slack`

---

### Firebase

Delivers push notifications via Firebase Cloud Messaging (legacy HTTP API).

```go
import "github.com/goliatone/go-notifications/pkg/adapters/firebase"

adapter := firebase.New(logger,
    firebase.WithConfig(firebase.Config{
        ServerKey: "AAAA...",
    }),
)
```

**Config struct**:

```go
type Config struct {
    ServerKey string        // FCM server key
    Endpoint  string        // FCM endpoint
    Timeout   time.Duration
    DryRun    bool
}
```

**Metadata fields**:

| Field | Description |
|-------|-------------|
| `token` | Device registration token |
| `topic` | Topic name |
| `condition` | Topic condition expression |
| `click_action` | Click action URL |
| `image` | Notification image URL |
| `data` | Custom data payload (map[string]any) |

**Channels**: `push`, `firebase`

---

### AWS SNS

Delivers SMS or topic messages via Amazon SNS.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/aws_sns"

adapter := aws_sns.New(logger,
    aws_sns.WithConfig(aws_sns.Config{
        Region:    "us-east-1",
        AccessKey: "AKIA...",
        SecretKey: "xxxxx",
        TopicARN:  "arn:aws:sns:us-east-1:123456789:my-topic",
    }),
)
```

**Config struct**:

```go
type Config struct {
    Region       string
    AccessKey    string
    SecretKey    string
    SessionToken string        // For temporary credentials
    TopicARN     string        // Default topic ARN
    DryRun       bool
    Timeout      time.Duration
}
```

**Metadata fields**: `topic_arn`, `body`

**Channels**: `sms`, `chat`

---

### Webhook

Posts notifications to any HTTP endpoint.

```go
import "github.com/goliatone/go-notifications/pkg/adapters/webhook"

adapter := webhook.New(logger,
    webhook.WithConfig(webhook.Config{
        URL:    "https://api.example.com/notify",
        Method: "POST",
        Headers: map[string]string{
            "X-API-Key": "secret",
        },
    }),
)
```

**Config struct**:

```go
type Config struct {
    URL             string
    Method          string            // Default: POST
    Headers         map[string]string
    Timeout         time.Duration
    SkipTLSVerify   bool
    BasicAuthUser   string
    BasicAuthPass   string
    DryRun          bool
    ForwardMetadata bool              // Include metadata in payload
    ForwardHeaders  bool              // Include headers in payload
}
```

**Payload format**:

```json
{
  "channel": "webhook",
  "to": "recipient",
  "subject": "Hello",
  "text": "Message body",
  "html": "<p>HTML body</p>",
  "metadata": {},
  "headers": {}
}
```

**Channels**: `webhook`, `chat`

---

## Secrets Management

Some adapters can receive credentials via metadata at runtime, enabling per-tenant or per-user secrets.
Adapters that currently read `msg.Metadata["secrets"]` are:
- SendGrid
- Twilio (SMS/WhatsApp)
- Telegram
- Slack

Example usage:

```go
// Secrets are injected into msg.Metadata["secrets"]
msg := adapters.Message{
    Metadata: map[string]any{
        "secrets": map[string][]byte{
            "api_key": []byte("tenant-specific-key"),
            "from":    []byte("tenant@example.com"),
        },
    },
}
```

Adapters that support secrets check credentials in this priority order:
1. `msg.Metadata["<key>"]` (explicit override)
2. `msg.Metadata["secrets"]["<key>"]` (runtime secret)
3. Adapter configuration default

See [GUIDE_SECRETS.md](GUIDE_SECRETS.md) for comprehensive secrets management.

---

## Writing Custom Adapters

### Basic Structure

```go
package myservice

import (
    "context"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

type Adapter struct {
    name string
    base adapters.BaseAdapter
    caps adapters.Capability
    cfg  Config
}

type Config struct {
    APIKey  string
    BaseURL string
}

type Option func(*Adapter)

func WithConfig(cfg Config) Option {
    return func(a *Adapter) { a.cfg = cfg }
}

func New(l logger.Logger, opts ...Option) *Adapter {
    adapter := &Adapter{
        name: "myservice",
        base: adapters.NewBaseAdapter(l),
        caps: adapters.Capability{
            Name:     "myservice",
            Channels: []string{"email", "sms"},
            Formats:  []string{"text/plain", "text/html"},
        },
    }
    for _, opt := range opts {
        if opt != nil {
            opt(adapter)
        }
    }
    return adapter
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Capabilities() adapters.Capability { return a.caps }

func (a *Adapter) Send(ctx context.Context, msg adapters.Message) error {
    // 1. Extract/validate configuration
    if a.cfg.APIKey == "" {
        return fmt.Errorf("myservice: api key required")
    }

    // 2. Build request payload
    body := msg.Body
    if htmlBody := msg.Metadata["html_body"]; htmlBody != nil {
        // Handle HTML content
    }

    // 3. Make API call
    // ...

    // 4. Log success
    a.base.LogSuccess(a.name, msg)
    return nil
}
```

### Registering Custom Adapters

```go
customAdapter := myservice.New(logger, myservice.WithConfig(myservice.Config{
    APIKey: "xxxxx",
}))

registry := adapters.NewRegistry(
    customAdapter,
    // ... other adapters
)
```

---

## Multi-Channel Fan-Out

Definitions can specify multiple channels for automatic fan-out:

```go
definition := &domain.NotificationDefinition{
    Code:         "order-shipped",
    Channels:     domain.StringList{"email:sendgrid", "sms:twilio", "push:firebase"},
    TemplateKeys: domain.StringList{
        "email:order-shipped-email",
        "sms:order-shipped-sms",
        "push:order-shipped-push",
    },
}
```

When triggered, the dispatcher:
1. Loads the definition
2. For each channel, finds the matching adapter
3. Renders the appropriate template
4. Delivers via each adapter in parallel

---

## Retry Policies

The dispatcher handles retries with exponential backoff by default and allows custom backoff strategies.

```go
cfg := config.DispatcherConfig{
    Enabled:    true,
    MaxRetries: 3,    // Retry up to 3 times
    MaxWorkers: 4,    // 4 concurrent workers
}
```

**Retry behavior**:
- Failed deliveries are retried up to `MaxRetries` times
- Each attempt is recorded in `DeliveryAttempt` table
- Default backoff is exponential (configurable)
- Final failure marks the message as failed

**Custom backoff**:

```go
backoff := retry.ExponentialBackoff{
    Base: 200 * time.Millisecond,
    Max:  5 * time.Second,
}

mod, err := notifier.NewModule(notifier.ModuleOptions{
    Config:  cfg,
    Backoff: backoff,
    // ...
})
```

**Delivery status flow**:
```
Pending → Delivered
       ↘
        Failed
```

---

## Troubleshooting

### "adapters: no adapter matches route"

No adapter is registered that matches the channel string:

```go
// Definition uses email:sendgrid
Channels: domain.StringList{"email:sendgrid"}

// Ensure SendGrid adapter is registered
registry := adapters.NewRegistry(
    sendgrid.New(logger, sendgrid.WithAPIKey("...")),
)
```

### "sendgrid: api key required"

Credentials are missing. Provide via:
1. Adapter configuration: `sendgrid.WithAPIKey("...")`
2. Runtime secrets: `msg.Metadata["secrets"]["api_key"]`

### "smtp: host is required"

SMTP adapter needs server configuration:

```go
smtp.New(logger,
    smtp.WithHostPort("smtp.example.com", 587),
    // ...
)
```

### Dry Run Mode

Most adapters support `DryRun` to test without actual delivery:

```go
twilio.New(logger, twilio.WithConfig(twilio.Config{
    DryRun: true,  // Log only, don't send
}))
```

### Debug Logging

Use the console adapter or enable structured logging to debug message flow:

```go
console.New(logger, console.WithStructured(true))
```

---

## Quick Reference

| Adapter | Channels | Auth Method |
|---------|----------|-------------|
| Console | email | None |
| SMTP | email | Username/Password |
| SendGrid | email | API Key |
| Mailgun | email | API Key |
| AWS SES | email | AWS Credentials |
| Twilio | sms, whatsapp | Account SID + Auth Token |
| WhatsApp | whatsapp, chat | Graph API Token |
| Telegram | chat | Bot Token |
| Slack | chat, slack | OAuth Token |
| Firebase | push, firebase | Server Key |
| AWS SNS | sms, chat | AWS Credentials |
| Webhook | webhook, chat | Headers / Basic Auth |
