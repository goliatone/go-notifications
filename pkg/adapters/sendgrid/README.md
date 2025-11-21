SendGrid Adapter
----------------
Delivers `email` channel messages through SendGrid’s REST API with text and HTML variants.

Usage
- Initialize with API key and default from: `sendgrid.New(logger, sendgrid.WithAPIKey("SG.x"), sendgrid.WithFrom("no-reply@example.com"))`.
- Optional: `sendgrid.WithReplyTo`, `WithBaseURL`, `WithTimeout`, `WithHTTPClient`.
- Per-message metadata: `from`, `reply_to`, `text_body`, `html_body`, `body`, `cc`, `bcc`.

Credentials
- Create an API key in SendGrid: https://app.sendgrid.com/settings/api_keys
- Sender identity/domain setup: https://docs.sendgrid.com/ui/account-and-settings/how-to-set-up-domain-authentication
