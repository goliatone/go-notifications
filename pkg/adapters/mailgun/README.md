Mailgun Adapter
---------------
Delivers `email` channel messages via Mailgun’s Messages API, supporting text, HTML, headers, cc/bcc, and optional attachments.

Usage
- Configure domain, API key, and from: `mailgun.New(logger, mailgun.WithConfig(mailgun.Config{Domain: "mg.example.com", APIKey: "key-xxx", From: "no-reply@example.com"}))`.
- Attachments: set `Message.Attachments` with `adapters.Attachment` content (metadata attachments still supported; URL-only attachments are ignored).
- Per-message metadata: `from`, `reply_to`, `text_body`, `html_body`, `body`, `cc`, `bcc`, `headers`, `attachments` (slice of maps: `filename`, `content` ([]byte), `content_type`).

Credentials
- Find your domain and API key in Mailgun dashboard: https://app.mailgun.com/app/account/security/api_keys
- Domain setup/verification: https://documentation.mailgun.com/en/latest/user_manual.html#verifying-your-domain
