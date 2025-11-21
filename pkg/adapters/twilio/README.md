Twilio Adapter
--------------
Delivers `sms` and `whatsapp` channel messages via Twilio’s Messaging API. Supports plain text; HTML is stripped to text.

Usage
- Configure credentials: `twilio.New(logger, twilio.WithConfig(twilio.Config{AccountSID: "ACxxx", AuthToken: "xxx", From: "+15551234567", MessagingServiceSID: "MGxxx"}))`.
- Optional `DryRun` to log but not send (or when creds are absent): `twilio.WithConfig(twilio.Config{DryRun: true, ...})`.
- Per-message metadata: `from`, `body`, `html_body` (stripped), `media_urls` ([]string), `messaging_service_sid`.
- WhatsApp: set channel to `whatsapp` and numbers in E.164; adapter adds `whatsapp:` prefix if missing.

Credentials
- Account SID/Auth Token: Twilio Console > Account Info: https://www.twilio.com/console
- Messaging Service SID & From numbers: https://www.twilio.com/console/sms/services
- WhatsApp senders: https://www.twilio.com/docs/whatsapp
