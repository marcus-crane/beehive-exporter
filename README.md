# beehive-exporter

A Go tool for archiving press releases from [beehive.govt.nz](https://www.beehive.govt.nz), the official website of the New Zealand Government.

## Installation

```bash
go install github.com/marcus-crane/beehive-exporter/cmd/beehive-exports@latest
```

## Usage

The tool has four main commands:

### archive
Build a discovery index from the search page:
```bash
beehive-exports archive                                  # Index all releases
beehive-exports archive --government 2023-2026-national  # Index one government
beehive-exports archive --pages 5                        # Index first 5 pages
beehive-exports archive --type speech                    # Index speeches instead
```

### fetch
Download raw HTML from the discovery index:
```bash
beehive-exports fetch                                    # Fetch all indexed releases
beehive-exports fetch --government 2023-2026-national    # Fetch one government
beehive-exports fetch --year 2024                        # Fetch by year
```

### process
Generate JSON and Markdown from raw HTML:
```bash
beehive-exports process            # Generate both JSON and Markdown
beehive-exports process --json     # Generate only JSON
beehive-exports process --markdown # Generate only Markdown
```

### reingest
Re-fetch and overwrite a single release:
```bash
beehive-exports reingest --id some-release-slug
beehive-exports reingest --url https://www.beehive.govt.nz/release/some-release
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BROWSERLESS_URL` | Base URL for Browserless instance (required for archive command) |
| `BROWSERLESS_TOKEN` | API token for Browserless |

## Output Structure

```
raw/{content-type}/YYYY/MM/YYYY-MM-DD-slug.html
json/{content-type}/YYYY/MM/YYYY-MM-DD-slug.json
markdown/{content-type}/YYYY/MM/YYYY-MM-DD-slug.md
discovery-index/{government}.json
index.json
url-index.json
```

Content types: `releases`, `speeches`, `features`, `diaries`

## Related Repositories

- [beehive-extracts](https://github.com/marcus-crane/beehive-extracts) - Raw HTML + JSON data
- [beehive-markdown](https://github.com/marcus-crane/beehive-markdown) - Markdown exports
