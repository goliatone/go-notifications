WhatsApp Adapter
----------------
Delivers `whatsapp` (and `chat`) channel messages via the Meta Cloud API. Supports plain text; HTML is stripped to text. Use Twilio adapter for Twilio-hosted WhatsApp numbers; use this adapter for direct Meta Cloud API.

Usage
- Configure token and phone number ID: `whatsapp.New(logger, whatsapp.WithConfig(whatsapp.Config{Token: "<bearer_token>", PhoneNumberID: "<phone_number_id>"}))`.
- Optional: `APIBase`, `Timeout`, `SkipTLSVerify`, `PlainOnly`, custom HTTP client.
- Per-message metadata: `body`, `html_body` (stripped), `preview_url` (bool) to enable link previews.
- Attachments: provide `Message.Attachments` with `URL` values; the first attachment is sent as a document with optional caption.

Credentials
- Create a WhatsApp business app in Meta for Developers: https://developers.facebook.com/docs/whatsapp/cloud-api/get-started
- Get a permanent access token and phone number ID from the Meta dashboard: https://developers.facebook.com/apps/ (WhatsApp > API Setup)
