Console Adapter
---------------
Logs rendered messages for the `email` channel to stdout / your configured logger. No credentials required.

Usage
- Register with `console.New(logger)` (optional: `console.WithStructured(true)` to log structured fields).
- Messages honor `text_body`, `html_body`, and `content_type` metadata for logging clarity.

When to use
- Local development and testing to inspect rendered payloads without sending real messages.
