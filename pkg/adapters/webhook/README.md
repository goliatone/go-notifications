Webhook Adapter
---------------
Posts rendered notifications to an HTTP endpoint as JSON. Supports text and HTML fields, and can forward headers/metadata.

Usage
- Configure endpoint and method (default POST):  
  `webhook.New(logger, webhook.WithConfig(webhook.Config{URL: "https://example.com/webhook"}))`
- Optional: custom headers, basic auth, timeout, dry-run, forward metadata/headers, custom HTTP client.
- Per-message metadata: `body`, `html_body`, in addition to `subject`, `to`, `channel`.

Payload (JSON)
```json
{
  "channel": "webhook",
  "to": "recipient-id-or-url",
  "subject": "Subject text",
  "text": "Plain text body",
  "html": "<p>HTML body</p>",
  "metadata": { ... },   // if enabled
  "headers":  { ... }    // if enabled
}
```

Credentials
- Depends on your endpoint. If the webhook requires auth, use basic auth config or custom headers (e.g., bearer tokens).
- No external credential provisioning is required by this adapter itself.
