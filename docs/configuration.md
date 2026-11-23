# Configuration

Almost everything is configured in the web interface and stored in the database. Only two things
are set when starting the program: **where the interface listens** and **where data is stored**.

## Commands

```
zvintedwatch [run] [options]            start ZVintedWatch (default command)
zvintedwatch reset-password [options]   replace the interface password with a random one
zvintedwatch debug search <url|words>   run one search and print the listings (no database)
zvintedwatch debug item <listing url>   print the description of a listing (no database)
zvintedwatch debug enrich <listing url> print the extra details of a listing, e.g. its store
zvintedwatch version                    print version information
```

## Options and environment variables

| Option | Environment variable | Default | Description |
|---|---|---|---|
| `-listen` | `ZVW_LISTEN` | `127.0.0.1:8080` | Address and port of the web interface. `127.0.0.1` only accepts connections from this computer; `0.0.0.0` accepts connections from the network (a password then becomes mandatory). |
| `-data-dir` | `ZVW_DATA_DIR` | `data` next to the executable | Folder holding the database and the log file. |
| `-open` | — | `true` on Windows, `false` elsewhere | Open the interface in the default browser at startup. |
| `-debug` | `ZVW_DEBUG` (any value) | off | Verbose logging. |

Options take precedence over environment variables. Examples:

```sh
zvintedwatch -listen 0.0.0.0:9000 -data-dir /srv/zvintedwatch
ZVW_LISTEN=127.0.0.1:18080 zvintedwatch
```

The `debug` commands detect the site from the URL. For plain keywords add `-provider <id>`
(`vinted`, `cashconverters`) and optionally `-domain` (e.g. `vinted.fr`). `-proxy socks5://…`
sends the requests through a proxy. They are useful to check quickly that a site module still
works.

## Data folder

| File | Content |
|---|---|
| `zvintedwatch.db` | SQLite database: alerts, Telegram chats and token, settings, password hash, history |
| `zvintedwatch.log` | Application log, rotated at 10 MB (`zvintedwatch.log.1` keeps the previous one) |

Old records are cleaned up automatically every hour:

| Data | Kept for |
|---|---|
| Listings already seen by each search | 14 days after they were last seen |
| Cached descriptions and “already sent” markers | 14 days |
| Match history | 90 days |

## Settings in the interface

### Telegram

Bot token, chats receiving alerts and the optional chat for system notices. See the
[Telegram guide](telegram-setup.md).

### Sites

Each site can be paused or resumed (*Settings → Sites*, or the switch on the site page). A paused
site sends no request; its alerts are kept unchanged. On resume, its searches record a new baseline
so listings posted during the pause are not sent.

### Network (per site)

| Setting | Default | Meaning |
|---|---|---|
| Minimum gap between requests | 3000 ms | Two requests to the same site through the same connection are never closer than this. |
| Random extra delay | 1500 ms | A random delay up to this value is added to every gap. |
| Request timeout | 30 s | Maximum duration of a request. |
| Proxies | none | One URL per line (`http://`, `https://`, `socks5://`, credentials allowed as `user:password@`). Requests are spread over the proxies, each paced separately and with its own session. |

These settings limit the *rate* of requests. The *number* of requests depends on your alerts:
each distinct search is refreshed at its own interval, shown on the site page and on the
dashboard (“requests/hour”).

### Security

When the interface listens on a non-local address, a password is required. On first start
ZVintedWatch generates one and prints it in the console/logs. You can change it in
*Settings → Security*; on a local-only setup you can also remove it. Changing the password logs
out every browser.

Forgot it? Stop ZVintedWatch and run:

```sh
zvintedwatch reset-password -data-dir <your data folder>
```

## Alert settings

| Field | Notes |
|---|---|
| Name | Shown in the interface and in Telegram messages. |
| Search | A search URL copied from the website, or keywords plus a country domain. URL filters are kept; the sort order is always “newest first”. |
| Title / description keywords | See [keyword syntax](keyword-syntax.md). |
| Price range | Optional, checked locally. |
| Refresh interval | Fixed or random range, in seconds. Vinted: minimum 10 s, 60 s or more recommended. Cash Converters: minimum 30 s, 2 min recommended. |
| Chats | Telegram destinations. Without any chat, matches are only listed in the interface. |
| Enabled | Paused alerts keep their settings but are not refreshed. |

Alerts whose searches are identical (same site, same parameters — keyword case and spacing do not
matter) are refreshed together with **one** request, at the pace of the most frequent of them:
the delay is drawn between the smallest minimum and the smallest maximum of their intervals.
