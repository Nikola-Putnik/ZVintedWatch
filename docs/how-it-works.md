# How it works

This page explains what ZVintedWatch does behind the scenes. It is useful if you want to tune it,
contribute, or simply understand why it behaves the way it does.

## Architecture

```
                         ┌──────────────────────────── zvintedwatch (one process) ───────────────────────────┐
 Browser ── HTTP/SSE ──▶ │  web          REST API, live event stream, embedded Svelte interface              │
                         │   │                                                                                │
                         │   ▼                                                                                │
                         │  engine ──── groups alerts by search, schedules refreshes, detects new listings,  │
                         │   │  ▲        evaluates keyword rules (matcher), records matches                  │
                         │   │  │                                                                             │
                         │   ▼  │                                                                             │
                         │  provider registry ── vinted (one package per marketplace)                        │
                         │   │                                                                                │
                         │   ▼                                                                                │
                         │  httpx ─── lanes (direct / proxies): pacing, cooldowns, browser fingerprint ────────┼──▶ marketplace
                         │                                                                                    │
                         │  notify/telegram ── per-chat queues, rate limits, retries ──────────────────────────┼──▶ Telegram Bot API
                         │  store ── SQLite (alerts, searches, seen listings, matches, settings)             │
                         │  activity ── event bus feeding the live console and the log file                  │
                         └────────────────────────────────────────────────────────────────────────────────────┘
```

| Package | Role |
|---|---|
| `cmd/zvintedwatch` | Command-line entry point (`run`, `reset-password`, `debug`, `version`). |
| `internal/app` | Builds and wires every component. New marketplaces are registered here. |
| `internal/config` | Command-line options and environment variables. |
| `internal/model` | Shared data types: alerts, listings, matches, chats, searches. |
| `internal/store` | SQLite persistence (pure Go, no CGO) with embedded migrations. |
| `internal/provider` | The `Provider` interface every marketplace module implements. |
| `internal/provider/vinted` | The Vinted module. |
| `internal/httpx` | Outgoing HTTP: lanes, pacing, cooldowns, Chrome fingerprint, cookie jars. |
| `internal/matcher` | Keyword rule compilation and matching. |
| `internal/engine` | Scheduling, new-listing detection, matching pipeline, previews. |
| `internal/notify/telegram` | Telegram client and delivery queues. |
| `internal/activity` | Event bus for the live console. |
| `internal/web` | HTTP API, Server-Sent Events, authentication, embedded interface. |
| `web/` | Svelte 5 + TypeScript sources of the interface (compiled into `internal/web/dist`). |

## The life of a refresh

1. **Grouping.** When alerts change, the engine parses every enabled alert's search into a
   normalized query. Alerts with the same query (same site, domain and parameters) form one
   **search job**.
2. **Scheduling.** Each job runs at a random delay between the smallest minimum and the smallest
   maximum interval of its alerts. Jobs never fire in bursts: they queue on the network lanes.
3. **Fetching.** The provider requests the first page of results, sorted newest first (48 listings
   on Vinted). One request, whatever the number of alerts.
4. **Filtering.** Promoted listings are dropped. Every listing id is recorded as *seen* for this
   search; ids never seen before are candidates.
5. **Baseline.** The first refresh of a search — or the first after a pause of more than
   30 minutes — only records what is there. Old listings are therefore never reported when you
   create an alert or restart the program.
6. **Bumped listings.** On Vinted, listing ids grow with time. A candidate whose id is far below the
   newest id already seen is an old listing pushed back to the top (paid “bump”, relisting): it is
   ignored.
7. **Matching.** For each candidate and each alert of the job: price range, then title rules. Only if
   they pass *and* the alert has description rules, the description is fetched (once per listing,
   cached and shared), then the description rules are applied.
8. **Notification.** Matches are recorded; one Telegram message per listing and per chat is queued,
   listing every matching alert. A chat never receives the same listing twice.

If a description cannot be read because of a temporary error, the listing is retried on the next
refreshes (up to three times) instead of being lost.

## Staying under the radar

Marketplaces protect themselves against automated traffic. ZVintedWatch combines several measures
to stay polite and look like a regular visitor:

- **Fewer requests**: shared searches, lazy and partial description reads (the item page is read
  only until the description is found — about 100 KB instead of 2 MB), previews that reuse recent
  results.
- **Pacing**: one request at a time per lane, with a minimum gap and random jitter between them;
  random refresh intervals.
- **Real browser fingerprint**: requests use the TLS and HTTP/2 fingerprint of Chrome
  (via [tls-client](https://github.com/bogdanfinn/tls-client)) with matching headers and header
  order, and each lane keeps its own cookie session, created once and renewed only when needed.
- **Backing off**: when a site answers “too many requests” (HTTP 429) or refuses access
  (HTTP 403), the lane pauses for 2 minutes, then 5, 15, 30 and 60 minutes if it happens again.
  Searches resume automatically; the site page shows the pause and offers a *Resume now* button.
- **Proxies**: optional, each with its own pace and session.

## Data stored

| Table | Purpose |
|---|---|
| `alerts` | Alert definitions. |
| `destinations` | Telegram chats. |
| `searches` | One row per active search job: baseline state, last refresh, highest listing id seen. |
| `seen_items` | Listing ids already returned by each search (pruned after 14 days without being seen). |
| `item_cache` | Descriptions already downloaded (14 days). |
| `matches` | Match history shown in the interface (90 days). |
| `deliveries` | Which listing was sent to which chat, to avoid duplicates (14 days). |
| `settings` | Telegram token, network settings, password hash, session secret. |

## Live console

Every component publishes events (request sent and its result, refresh summary, match, Telegram
delivery, pauses) on an in-memory bus. The last 1,500 events are kept and streamed to the browser
with Server-Sent Events; they are also written to the log file. Credentials are never shown:
proxy passwords are hidden and session tokens are not part of the displayed URLs.
