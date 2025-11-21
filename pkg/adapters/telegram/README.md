Telegram Adapter
----------------
Delivers `chat` channel messages via the Telegram Bot API with optional HTML (via parse_mode=HTML) or plain text.

Usage
- Configure token: `telegram.New(logger, telegram.WithConfig(telegram.Config{Token: "<bot_token>"}))`.
- Optional: `ParseMode`, `DisableWebPagePreview`, `DisableNotification`, custom `BaseURL`, `Timeout`.
- Per-message metadata: `html_body`, `body`, `parse_mode`, `disable_preview`, `silent`, `thread_id`, `reply_to`.

Credentials
- Create a bot with @BotFather to obtain the token: https://core.telegram.org/bots#3-how-do-i-create-a-bot
- Telegram Bot API docs: https://core.telegram.org/bots/api#sendmessage
