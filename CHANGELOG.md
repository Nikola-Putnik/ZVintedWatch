# Changelog

All notable changes to this project are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/).

## [Unreleased]

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
- Single binary for Windows, Linux (amd64, arm64, armv7) and macOS, Docker image, systemd units.
