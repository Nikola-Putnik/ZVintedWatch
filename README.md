# ZVintedWatch

**Get a Telegram message the moment a matching item is listed on Vinted (and other second-hand marketplaces).**

ZVintedWatch is a self-hosted listing monitor. You describe what you are looking for (a search link or keywords, plus words that must or must not appear in the title or description), and it watches the marketplace for you, sending new matches to a Telegram chat or channel.

> 🚧 Work in progress — the first release is being built. See the roadmap below.

## Planned features

- Multi-site architecture (one module per marketplace) — Vinted first.
- Custom alerts: search link or keywords, title/description include & exclude keywords, price range.
- Fixed or randomized refresh interval per alert.
- Telegram notifications to a private chat or a channel.
- Clean web interface with one section per site and a live request console.
- Request-saving engine: alerts sharing the same search share a single request, descriptions are only fetched when the title already matches.
- Single executable for Windows, Linux (servers, seedboxes, Raspberry Pi) and macOS.

## License

[MIT](LICENSE)
