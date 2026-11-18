# Contributing

Thanks for your interest in ZVintedWatch! Bug reports, documentation fixes, new marketplace
modules and improvements are welcome.

## Development setup

Requirements:

- [Go](https://go.dev/dl/) 1.27 or newer
- [Node.js](https://nodejs.org/) 20 or newer (only to build the web interface)

```sh
git clone https://github.com/Nikola-Putnik/ZVintedWatch.git
cd ZVintedWatch
npm --prefix web ci
```

### Running in development

Run the backend (it serves the API on port 8080):

```sh
go run ./cmd/zvintedwatch -data-dir ./data
```

In a second terminal, run the interface with hot reload; it forwards API calls to the backend:

```sh
npm --prefix web run dev
# open http://localhost:5173
```

The `data/` folder is ignored by git. Use `-debug` for verbose logs.

### Building

```sh
./scripts/build.sh                      # Linux/macOS
powershell -File scripts\build.ps1      # Windows
```

The web interface is compiled into `internal/web/dist` and embedded in the Go binary
(`go:embed`). Without a build of the interface, the binary still works and shows a page explaining
how to build it.

## Checks

Run these before opening a pull request (CI runs them too):

```sh
gofmt -l .                 # must print nothing
go vet ./...
go test ./...              # add -race on Linux/macOS
npm --prefix web run check # TypeScript and Svelte checks
npm --prefix web run build
```

## Project layout

See [docs/how-it-works.md](docs/how-it-works.md#architecture) for the role of each package, and
[docs/adding-a-provider.md](docs/adding-a-provider.md) to add a marketplace.

## Conventions

- **Language**: code, comments, documentation, interface texts and commit messages are in English.
- **Go**: standard formatting (`gofmt`), small packages with a package comment, exported
  identifiers documented, errors wrapped with context (`fmt.Errorf("…: %w", err)`).
- **Frontend**: Svelte 5 runes, TypeScript, no UI framework; styles use the CSS variables of
  `web/src/app.css` so light and dark themes keep working.
- **Commits**: [Conventional Commits](https://www.conventionalcommits.org/) style
  (`feat(vinted): …`, `fix(engine): …`, `docs: …`).
- **Tests**: new behaviour comes with tests. Fixtures must be **synthetic**: never commit real
  marketplace responses, listing pages, tokens or chat ids — they contain other people's data.
- **Politeness**: changes must not increase the number of requests sent to marketplaces without a
  good reason; explain it in the pull request when they do.

## Releasing

Maintainers publish a release by pushing a tag:

```sh
git tag v1.2.0
git push origin v1.2.0
```

The release workflow builds the archives for every platform with GoReleaser and publishes the
Docker image to GitHub Container Registry. Update [CHANGELOG.md](CHANGELOG.md) first.
