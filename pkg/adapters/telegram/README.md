Telegram Adapter
----------------
Delivers `chat` channel messages via the Telegram Bot API with optional HTML (via parse_mode=HTML) or plain text.

Usage
- Configure token: `telegram.New(logger, telegram.WithConfig(telegram.Config{Token: "<bot_token>"}))`.
- Optional: `ParseMode`, `DisableWebPagePreview`, `DisableNotification`, custom `BaseURL`, `Timeout`.
- Per-message metadata: `html_body`, `body`, `parse_mode`, `disable_preview`, `silent`, `thread_id`, `reply_to`.

Credentials
- Create a bot with @BotFather to obtain the token: https://core.telegram.org/bots#how-do-i-create-a-bot
- Telegram Bot API docs: https://core.telegram.org/bots/api#sendmessage
- Get a chat ID:
  - For a 1:1 chat, send any message to your bot, then call `https://api.telegram.org/bot<token>/getUpdates` and read `message.chat.id`.
  - For a group, add the bot and send `/start`, then call `getUpdates` and use the `chat.id` from the group message. Supergroups use negative IDs (e.g., `-1001234567890`); keep the sign intact.
