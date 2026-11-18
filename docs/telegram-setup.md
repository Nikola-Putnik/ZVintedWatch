# Telegram setup

ZVintedWatch sends notifications through **your own Telegram bot**. A bot is free and takes two
minutes to create. You then tell ZVintedWatch which **chats** should receive the alerts: your
private chat with the bot, a group, or a channel.

## 1. Create the bot

1. In Telegram, open a conversation with [@BotFather](https://t.me/BotFather) (the official bot
   used to create bots).
2. Send `/newbot`.
3. Choose a display name (e.g. `My deals`) and a username that ends with `bot`
   (e.g. `my_deals_watch_bot`).
4. BotFather answers with a **token** that looks like `123456789:AAHk3…`. Keep it secret: anyone
   with this token can control the bot.

## 2. Connect it to ZVintedWatch

Open **Settings → Telegram bot**, paste the token and click **Connect bot**. ZVintedWatch checks
the token with Telegram and shows the bot's username when it is valid.

The token is stored only in the local database (`data/zvintedwatch.db`).

## 3. Choose where alerts go

In **Settings → Chats receiving alerts**, click **Add chat**. Each chat has a name (only used in
the interface) and a **chat id**. The **Find chats** button lists the chats that recently talked
to your bot, so you rarely need to type an id.

### Your private chat

1. Open your bot in Telegram (search for its username) and press **Start** (or send any message).
2. In ZVintedWatch, click **Find chats**, pick your name, then **Save chat**.
3. Click **Test** to receive a test message.

Private chat ids are positive numbers, e.g. `123456789`.

### A channel (to share alerts with several people)

1. Create a channel in Telegram (public or private).
2. Open the channel settings → **Administrators** → **Add administrator** → choose your bot and keep
   the **Post messages** permission.
3. Post any message in the channel.
4. In ZVintedWatch, click **Find chats** and pick the channel.

Channel ids look like `-1001234567890`. For a public channel you can also simply type its public
name, e.g. `@my_deals_channel`.

### A group

1. Add the bot to the group.
2. Send `/start@your_bot_username` in the group (by default bots only see commands in groups).
3. Click **Find chats** and pick the group.

## Choosing chats per alert

Each alert has a **Send matches to** section where you tick one or more chats. A listing that
matches several alerts is sent **only once per chat**, with the names of all the matching alerts.

## System notices

Optionally, pick a chat in **Settings → System notices** to be told when a marketplace starts
refusing requests and ZVintedWatch pauses for a while (see
[how it works](how-it-works.md#staying-under-the-radar)).

## What a notification looks like

Each notification contains the listing photo, its title, the price, brand, size and condition,
the names of the matching alerts and a button that opens the listing.

Photos are converted to JPEG before being sent, because marketplaces often use the WebP format
that some Telegram apps do not display. If the photo cannot be sent, ZVintedWatch falls back to a
text message.

## Troubleshooting

| Problem | Solution |
|---|---|
| “The bot token was rejected” | Copy the whole token from BotFather again (`digits:letters`). Use `/token` in BotFather to see it. |
| Find chats returns nothing | Send a message to the bot (or post in the channel) **after** connecting the token, then retry. Updates older than 24 hours are dropped by Telegram. |
| “This bot uses a webhook” | The bot is used by another service with a webhook. Create a dedicated bot for ZVintedWatch. |
| “chat not found” | Check the chat id; for a private chat, press **Start** in the bot first. |
| “bot was blocked by the user” | Unblock the bot in Telegram. |
| “not enough rights to send” | Make the bot an administrator of the channel with the *Post messages* permission. |
| Alerts appear in the interface but not in Telegram | Make sure the alert has at least one chat ticked, and look at the **Live activity** page filtered on *Telegram* for errors. |

Telegram limits how fast a bot can post (about one message per second per chat, and 20 per minute
in groups and channels). ZVintedWatch queues messages and respects these limits automatically.
