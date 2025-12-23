SMTP Adapter
------------
Sends `email` channel messages via SMTP with text and HTML (multipart/alternative) support.

Usage
- Configure host/port/auth/from: `smtp.New(logger, smtp.WithHostPort("smtp.example.com", 587), smtp.WithCredentials("user", "pass"), smtp.WithFrom("no-reply@example.com"))`.
- Toggle TLS/STARTTLS: `smtp.WithTLS(true)` for implicit TLS (e.g., port 465) or `smtp.WithStartTLS(true)` (default) for STARTTLS.
- Optionally set metadata per message: `from`, `text_body`, `html_body`, `content_type`, `headers`, `text`, `body`.
- Attachments: set `Message.Attachments` with `adapters.Attachment` (SMTP builds a multipart/mixed email).

Credentials
- Obtain SMTP hostname, port, username/password, and sender address from your email provider (e.g., your ESP, a transactional email service, or your own mail server).
- Common docs: SendGrid SMTP https://docs.sendgrid.com/for-developers/sending-email/integrating-with-the-smtp-api , Mailgun SMTP https://documentation.mailgun.com/en/latest/user_manual.html#smtp-settings .
