# Changelog

All notable changes to this project are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-11-23

### Added

- Cash Converters Belgium module (cashconverters.be, French and Dutch): search and category pages
  with store and price filters, one request per refresh (newest items first), free description
  rules, and the store holding the item shown in alerts and in the match history.
- Pause or resume a whole site from its page or from *Settings → Sites*; its alerts are kept and
  other sites are not affected.
- Optional enrichment hook for site modules: details only available on the listing page are
  fetched once per matched listing.
- Listings can carry a category and a location, shown in Telegram messages and match cards.
- The debug command detects the site from the URL; new `debug enrich` command.

### Changed

- A crash inside a site module no longer stops the application; it is reported as a refresh error.
- Alerts of a site whose module is not installed are kept and reported on the dashboard.
- Automatic pauses after refused requests are labelled “cooling down”.

## [0.1.0] - 2026-11-18

### Added

- First version of ZVintedWatch.
- Vinted module: search by URL (with filters) or keywords on 23 country domains, anonymous
  session, listing details (brand, size, condition, photo), description reading from item pages.
- Alerts with title and description keyword rules (phrases, alternatives, wildcards, regular
  expressions, exclusions), price range, fixed or random refresh interval and Telegram chats.
- Request-saving engine: alerts on the same search share one request, descriptions are fetched
  lazily and only partially, previews reuse recent results.
- New-listing detection with a silent baseline, promoted-listing filtering and detection of old
  listings bumped back to the top of the results.
- Telegram delivery to private chats, groups and channels with per-chat rate limiting, retries,
  duplicate protection and chat discovery.
- Network layer with a Chrome TLS fingerprint, request pacing, automatic cooldowns and optional
  HTTP/SOCKS5 proxies.
- Web interface: dashboard, one section per site, alert editor with live keyword tester and
  preview, live activity console, settings, light/dark themes, mobile layout.
- Optional password (mandatory when the interface is reachable from the network).
- Single binary for Windows (amd64, arm64), Linux (amd64, arm64, armv7) and macOS, Docker image,
  systemd units.

[Unreleased]: https://github.com/Nikola-Putnik/ZVintedWatch/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Nikola-Putnik/ZVintedWatch/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Nikola-Putnik/ZVintedWatch/releases/tag/v0.1.0
