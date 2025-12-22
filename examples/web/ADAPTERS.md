# Notification Adapters Configuration

The web example automatically detects and enables notification adapters based on environment variables. No code changes are needed to test different adapters.

## Quick Start

1. Set environment variables for the adapter(s) you want to test
2. Run the example: `go run .`
3. The enabled adapters will be logged on startup
4. Admin panel will show which adapters are active

## Data & Secrets
- SQLite file `tmp/demo.db` stores definitions, templates, messages, preferences, contacts, delivery logs, and encrypted secrets.
- Demo contacts are seeded for Alice, Bob, and Carlos (email, phone, Slack IDs, Telegram chat IDs).
- Fake tokens/API keys are written into the encrypted secrets store so adapters can run without env vars; set `DEMO_SECRET_KEY` to change the local encryption key.
- Per-user provider choices (e.g., Slack vs Telegram for `chat`) are editable in the Preferences panel and persisted to SQLite.

## Available Adapters

### Console (Always Enabled)
Logs notifications to stdout. No configuration required.

### Slack

**Environment Variables:**
```bash
export SLACK_TOKEN="xoxb-your-bot-token"
export SLACK_CHANNEL="#notifications"  # Default: #notifications
```

**How to get credentials:**
1. Create a Slack App at https://api.slack.com/apps
2. Add Bot Token Scopes: `chat:write`, `chat:write.public`
3. Install app to workspace
4. Copy the Bot User OAuth Token

### Telegram

**Environment Variables:**
```bash
export TELEGRAM_BOT_TOKEN="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
export TELEGRAM_CHAT_ID="-1001234567890"
```

**How to get credentials:**
1. Create bot via @BotFather on Telegram
2. Get bot token from BotFather
3. Add bot to group/channel
4. Get chat ID using: https://api.telegram.org/bot<TOKEN>/getUpdates

### Twilio (SMS)

**Environment Variables:**
```bash
export TWILIO_ACCOUNT_SID="ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
export TWILIO_AUTH_TOKEN="your-auth-token"
export TWILIO_FROM_PHONE="+1234567890"
```

**How to get credentials:**
1. Sign up at https://www.twilio.com
2. Get Account SID and Auth Token from console
3. Get or buy a phone number

### SendGrid (Email)

**Environment Variables:**
```bash
export SENDGRID_API_KEY="SG.xxxxxxxxxxxxxxxxxx"
export SENDGRID_FROM_EMAIL="noreply@example.com"
export SENDGRID_FROM_NAME="Notification System"  # Optional
```

**How to get credentials:**
1. Sign up at https://sendgrid.com
2. Create API key with "Mail Send" permission
3. Verify sender email address

### Mailgun (Email)

**Environment Variables:**
```bash
export MAILGUN_API_KEY="key-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
export MAILGUN_DOMAIN="mg.example.com"
export MAILGUN_FROM_EMAIL="noreply@example.com"
export MAILGUN_FROM_NAME="Notification System"  # Optional
```

**How to get credentials:**
1. Sign up at https://www.mailgun.com
2. Add and verify domain
3. Get API key from dashboard

### WhatsApp (via Twilio)

**Environment Variables:**
```bash
export WHATSAPP_ACCOUNT_SID="ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
export WHATSAPP_AUTH_TOKEN="your-auth-token"
export WHATSAPP_FROM_PHONE="whatsapp:+1234567890"
```

**How to get credentials:**
1. Enable WhatsApp on Twilio account
2. Use same credentials as Twilio SMS
3. Prefix phone number with `whatsapp:`

## Example Configurations

### Test with Slack only:
```bash
export SLACK_TOKEN="xoxb-123..."
export SLACK_CHANNEL="#test-notifications"
go run .
```

### Test with multiple adapters:
```bash
export SLACK_TOKEN="xoxb-123..."
export SLACK_CHANNEL="#notifications"
export TELEGRAM_BOT_TOKEN="123456:ABC..."
export TELEGRAM_CHAT_ID="-100123..."
export TWILIO_ACCOUNT_SID="AC123..."
export TWILIO_AUTH_TOKEN="abc123..."
export TWILIO_FROM_PHONE="+15551234567"
go run .
```

## How It Works

1. **Detection**: On startup, the app scans environment variables (optional) and enables matching adapters.
2. **Contacts & Secrets**: User contact data plus fake provider tokens/API keys are stored in SQLite and resolved per delivery.
3. **Dynamic Channels**: Notification definitions automatically include channels for active adapters.
4. **Templates**: Templates are seeded for all possible channels (unused ones are ignored).
5. **UI Updates**: Preferences include provider dropdowns; a “Last Delivery” panel shows which provider was used.

## Verification

When the app starts, you'll see output like:
```
INFO Enabled adapters (3): [console slack telegram]
INFO Available channels: [in-app console chat slack]
```

In the admin panel (login as alice@example.com), you'll see active adapter badges.

## Testing

1. Login as Alice (admin user)
2. Check "Active Adapters" section in admin panel
3. Use "Send to User" to send a test message
4. Or click "Send Test Notification" to send to yourself
5. Check configured channels (Slack, Telegram, etc.) for the notification

## Troubleshooting

**No adapters showing:**
- Verify environment variables are set: `env | grep SLACK`
- Check logs for configuration errors
- Ensure tokens/credentials are valid

**Notifications not delivered:**
- Check adapter has proper permissions
- Verify phone numbers/emails/channels are correct
- Look for errors in console logs
- Try sending directly via adapter API to isolate issue

## Channel Mapping

| Adapter   | Channel Type | Used For |
|-----------|--------------|----------|
| Console   | console      | Development/debugging |
| In-app    | in-app       | Web UI inbox |
| Slack     | slack, chat  | Team messaging |
| Telegram  | chat         | Direct messaging |
| Twilio    | sms          | Text messages |
| SendGrid  | email        | Transactional email |
| Mailgun   | email        | Transactional email |
| WhatsApp  | whatsapp     | WhatsApp messaging |
