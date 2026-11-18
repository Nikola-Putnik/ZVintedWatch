# Vinted module

Source: [`internal/provider/vinted`](../../internal/provider/vinted). This page documents how the
module talks to Vinted, for maintainers. Vinted has no public API; the module uses the same
private endpoints as the website, which **change from time to time**. Everything that depends on
them lives in this package.

## Requests

| Step | Request | Notes |
|---|---|---|
| Session | `HEAD https://www.vinted.<tld>/` | Vinted sets an anonymous session in cookies: `access_token_web` (a JWT) and `anon_id`. A HEAD request is enough and downloads nothing. The session is reused until the JWT is about to expire or a request is refused (401/403), then renewed once. |
| Search | `GET https://api.vinted.<tld>/svc-catalogue/items?…&order=newest_first&page=1&per_page=48` | Headers: `Authorization: Bearer <access_token_web>`, `x-anon-id: <anon_id>`, plus the headers of a cross-site XHR from `www.vinted.<tld>`. |
| Description | `GET https://www.vinted.<tld>/items/<id>-<slug>` | The HTML page contains a JSON-LD `Product` block with the full description. The body is streamed and reading stops as soon as the block is complete (≈100 KB of ≈2 MB). Fallback: the `og:description` meta tag (may be truncated). |

Since September 2026 the former endpoint `www.vinted.<tld>/api/v2/catalog/items` answers
`{"code":104}`, and `/api/v2/items/<id>/details` is protected by DataDome; neither is used.

## Search parameters

Users paste website URLs. The website and the catalogue API do not use the same parameter names,
so `query.go` translates them. Values of the same filter are comma-joined.

| Website parameter | API parameter |
|---|---|
| `search_text` | `search_text` (lowercased, spaces collapsed) |
| `catalog[]`, `catalog_ids[]`, path `/catalog/<id>-…` | `attribute_ids[catalog]` |
| `brand_ids[]`, path `/brand/<id>-…` | `attribute_ids[brand]` |
| `size_ids[]` | `attribute_ids[size]` |
| `status_ids[]` | `attribute_ids[status]` |
| `color_ids[]` | `attribute_ids[color]` |
| `material_ids[]` | `attribute_ids[material]` |
| `video_game_platform_ids[]` | `attribute_ids[video_game_platform]` |
| any other `<name>_ids[]` | `attribute_ids[<name>]` |
| `attribute_ids[<name>]` | kept as is |
| `price_from`, `price_to`, `currency` | kept as is |
| `country_ids[]`, `city_ids[]` | `country_ids`, `city_ids` |
| `order`, `page`, `per_page`, `time`, `search_id`, … | dropped (sorting and paging are set by the module) |

**Pitfalls:** the API silently ignores unknown parameter names and returns *unfiltered* results
(HTTP 200), and it rejects empty values (HTTP 400). Empty values are therefore dropped and ids are
de-duplicated and sorted, which also gives equivalent searches the same key so that their alerts
share one request.

## Listing fields

| API field | Listing |
|---|---|
| `id` | `ID`, and `Sequence` (ids grow over time) |
| `title` | `Title` |
| `price.amount`, `price.currency_code` | `Price`, `Currency` (item price without buyer fees) |
| `item_box.first_line` | `Brand` (when different from the title) |
| `item_box.second_line` | `Size` and `Condition` (`"M · Very good"`; no size for some categories) |
| `photo.url` | `PhotoURL` (WebP, converted to JPEG before being sent to Telegram) |
| `url` (relative) | `URL` (absolute) |
| `promoted` or `content_source` containing `promoted` | `Promoted` (ignored by the engine) |

The response no longer carries a creation date. To ignore old listings that are “bumped” back to
the top of newest-first results, the engine compares listing ids: a listing more than
`SequenceTolerance` ids (1.5 million, roughly one to two hours of new listings) below the highest
id already seen by the search is not treated as new.

## Anti-bot protection

The search API is behind Cloudflare and item pages behind DataDome. Plain HTTP clients are often
refused, especially from datacenter IP addresses. The module relies on `internal/httpx` (Chrome
TLS fingerprint, consistent headers, pacing, cooldowns). A 403 that persists after renewing the
session puts the lane in cooldown.

## When Vinted changes something

1. Reproduce with the debug command, which prints every request and the parsed listings:
   ```sh
   zvintedwatch debug search "https://www.vinted.fr/catalog?search_text=nike&brand_ids[]=53"
   zvintedwatch debug item "https://www.vinted.fr/items/123456789-something"
   ```
2. Open the same search in a browser with the developer tools (*Network* tab, filter *Fetch/XHR*)
   to see the request the website makes and its response.
3. Update `search.go` (endpoint, headers, response fields), `query.go` (parameter mapping),
   `session.go` (cookies/headers) or `item.go` (description extraction).
4. Update the tests in `vinted_test.go`. Fixtures are synthetic: **never commit real responses**,
   they contain other people's data.

## Supported domains

`vinted.at`, `.be`, `.co.uk`, `.com`, `.cz`, `.de`, `.dk`, `.es`, `.fi`, `.fr`, `.gr`, `.hr`,
`.hu`, `.ie`, `.it`, `.lt`, `.lu`, `.nl`, `.pl`, `.pt`, `.ro`, `.se`, `.sk` — see `domains.go`.
