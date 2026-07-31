# OnReady Notification Guide

This guide covers how to register the OnReady definition/templates and send
notifications via the `OnReadyNotifier`. The assets are opt-in and live in
`pkg/onready`.

## Register the assets

```go
translator, _ := i18n.NewSimpleTranslator(
    i18n.NewStaticStore(onready.Translations()),
    i18n.WithTranslatorDefaultLocale("en"),
)

tplRepo := memstore.NewTemplateRepository()
tplSvc, _ := templates.New(templates.Dependencies{
    Repository:    tplRepo,
    Cache:         &cache.Nop{},
    Logger:        &logger.Nop{},
    Translator:    translator,
    Fallbacks:     i18n.NewStaticFallbackResolver(),
    DefaultLocale: "en",
})

defRepo := memstore.NewDefinitionRepository()

result, err := onready.Register(ctx, onready.Dependencies{
    Definitions: defRepo,
    Templates:   tplSvc,
}, onready.Options{
    Namespace: "billing", // optional
})
// result.DefinitionCode is the code to send with
```

`Options` allows CTA label/icon overrides, namespace prefixes, and channel
filtering. Registration is idempotent; re-running will not duplicate records.

## Send via `OnReadyNotifier`

```go
mrg, _ := notifier.New(notifier.Dependencies{
    Definitions: defRepo,
    Events:      evtRepo,
    Messages:    msgRepo,
    Attempts:    attRepo,
    Templates:   tplSvc,
    Adapters:    adapters.NewRegistry(console.New(&logger.Nop{})),
    Logger:      &logger.Nop{},
    Config:      config.DispatcherConfig{},
    Inbox:       inboxSvc, // required if you use in-app
})

exp, _ := onready.NewNotifier(mrg, result.DefinitionCode)

_ = exp.Send(ctx, onready.OnReadyEvent{
    Recipients: []string{"user-1"},
    Locale:     "en",
    FileName:   "orders.csv", // still validated/used by templates
    Format:     "csv",
    URL:        "https://example.com/orders.csv",
    ExpiresAt:  "2025-01-01T00:00:00Z",
    Rows:       1200,
    Parts:      3,
    ManifestURL: "https://example.com/manifest.json",
    Message:    "Filtered by team",
    Attachments: []adapters.Attachment{
        {
            Filename:    "orders-summary.txt",
            ContentType: "text/plain",
            Content:     []byte("Run completed."),
        },
    },
    ChannelOverrides: map[string]map[string]any{
        "email": {
            "cta_label":  "Download now",
            "action_url": "https://cdn.example.com/orders.csv",
        },
    },
})
```

Per-channel overrides (`channel_overrides`) can adjust the CTA label, action
URL, body, subject, icon, or badge; defaults are provided in the templates.
The stored templates expect snake_case placeholders (`file_name`, `url`, etc.),
which the helper maps from the struct fields above when rendering.

### Attachments

Use `OnReadyEvent.Attachments` to include file payloads for email-capable
adapters (SMTP, Mailgun). URL-only channels (Slack, SMS/MMS, WhatsApp,
Telegram) read attachment URLs instead of raw bytes. Provide
`ChannelAttachments` to override attachments per channel.

```go
_ = exp.Send(ctx, onready.OnReadyEvent{
    // ...
    Attachments: []adapters.Attachment{
        {
            Filename:    "report.pdf",
            ContentType: "application/pdf",
            Content:     pdfBytes,
        },
    },
    ChannelAttachments: map[string][]adapters.Attachment{
        "sms": {
            {
                Filename: "report.pdf",
                URL:      "https://cdn.example.com/report.pdf",
            },
        },
    },
})
```

If you send raw `Content` for URL-only channels, configure an attachment
resolver/uploader on the dispatcher so bytes can be uploaded and converted to
URLs before delivery.

## Integration notes

- Call `onready.Register` during service bootstrap to ensure the ready
  definition/templates exist; pass a namespace to avoid collisions with other
  modules.
- Populate `OnReadyEvent` with optional `ManifestURL`/`Parts` when multipart
  artifacts exist; use `ChannelOverrides` to point CTAs at signed URLs/CDN
  endpoints if they differ from the stored URL.
- Inject `OnReadyNotifier` via DI so callers do not import internal packages;
  rely on the returned `DefinitionCode` from registration.
