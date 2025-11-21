Slack Adapter
-------------
Delivers `chat` / `slack` channel messages via Slack’s `chat.postMessage` API.

Usage
- Configure token and default channel: `slack.New(logger, slack.WithConfig(slack.Config{Token: "xoxb-...", Channel: "#alerts"}))`.
- Optional: `BaseURL`, `Timeout`, `SkipTLSVerify`, `DryRun`, custom HTTP client.
- Per-message metadata: `channel` (override), `body`, `html_body` (stripped to text), `thread_ts` (reply in thread).
- Set message channel to `slack` (or `chat`) in definitions.

Credentials
- Create a Slack app, add Bot Token scopes (e.g., `chat:write`), install to workspace to obtain a Bot User OAuth token.
- App creation & tokens: https://api.slack.com/apps
- chat.postMessage docs: https://api.slack.com/methods/chat.postMessage
