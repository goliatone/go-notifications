Firebase Adapter
----------------
Delivers `push` / `firebase` channel messages via Firebase Cloud Messaging (legacy HTTP API) using a server key.

Usage
- Configure with your FCM server key:  
  `firebase.New(logger, firebase.WithConfig(firebase.Config{ServerKey: "<FCM server key>"}))`
- Optional: `Endpoint`, `Timeout`, `DryRun`, custom HTTP client.
- Per-message metadata:
  - `token` (override `msg.To`), or `topic` (e.g., `news`), or `condition` for logical topic expressions.
  - `body`, `html_body` (HTML is added to data payload), `click_action`, `image`.
  - `data` (map[string]any) merged into the FCM data payload.

Credentials
- Use the FCM server key from Firebase Console > Project Settings > Cloud Messaging (Legacy server key).
- Docs: https://firebase.google.com/docs/cloud-messaging/http-server-ref
  (For the HTTP v1 API with OAuth2, you’d need service account credentials and JWT-based access tokens; this adapter targets the legacy key-based API for simplicity.)
