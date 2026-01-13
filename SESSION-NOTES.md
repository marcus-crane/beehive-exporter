# Beehive Exports - Session Notes

## Project Overview

CLI tool to export NZ government press releases from beehive.govt.nz. Fetches releases, stores as JSON, and can generate markdown files.

Uses Browserless (headless browser) to bypass Incapsula WAF blocking.

## Commands

```bash
# Build
go build -o beehive-exports ./cmd/beehive-exports

# Build discovery index (finds all release URLs from listing pages)
./beehive-exports index

# Fetch releases from discovery index
./beehive-exports fetch                    # All missing releases
./beehive-exports fetch --year 2024        # Specific year
./beehive-exports fetch --month 2024-03    # Specific month

# Sync from listing pages (alternative to index+fetch)
./beehive-exports sync
./beehive-exports sync --pages 10          # Limit pages

# Re-fetch a single release
./beehive-exports reingest --id some-release-slug
./beehive-exports reingest --url https://www.beehive.govt.nz/release/some-release

# Generate markdown from stored releases
./beehive-exports markdown
./beehive-exports markdown --out ./custom-dir
```

## Current State (2026-01-13)

### What's Done
- Discovery index built: **3412 releases** found (Nov 2023 - Jan 2026)
- All of **2024** fetched: 1477 releases
- **Jun 2025 - Jan 2026** already had: ~999 releases
- Total stored: ~2476 releases

### Data Locations
- JSON releases: `content/json/YYYY/MM/*.json`
- Raw HTML backups: `content/raw/YYYY/MM/*.html` (gitignored)
- Markdown output: `content/markdown/YYYY/MM/*.md`
- Discovery index: `content/discovery-index.json`
- Content index: `content/index.json`
- URL index: `content/url-index.json`

## What's Left To Do

### Immediate
1. Fetch early 2025 (Jan-May):
   ```bash
   ./beehive-exports fetch --year 2025
   ```
   This will skip existing Jun-Dec and fetch ~701 missing releases.

2. Regenerate markdown:
   ```bash
   ./beehive-exports markdown
   ```

### Future
- **Archive access**: The beehive has an archive with previous governments' releases (pre-Nov 2023). The current `/releases` listing only shows the current government. Need to explore the archive structure to index historical releases.
- User mentioned: "If you check Advanced Search, there are better labels https://www.beehive.govt.nz/advanced_search"

## Technical Notes

### Browserless
- Endpoint: `https://browser.home.utf9k.net`
- Token loaded from `.env` file (BROWSERLESS_TOKEN)
- Required to bypass Incapsula WAF on pagination

### Rate Limiting
- 1 second between requests (configurable in `internal/fetcher/fetcher.go`)

### Storage Optimization
- Only stores `div.ds-three-col__main` content (not full HTML)
- Reduced storage by ~97% (from ~350KB to ~10KB per release)
- Full HTML saved separately in `content/raw/` for reprocessing

### Markdown Generation
- Uses `github.com/JohannesKaufmann/html-to-markdown/v2`
- Front matter includes: title, date, url, ministers, portfolios, attachments

### Checkpointing
- Each release saved immediately after fetch
- Fetch command checks `ReleaseExists()` and skips saved releases
- Safe to interrupt and resume

## Key Files
- `cmd/beehive-exports/main.go` - CLI commands
- `internal/fetcher/fetcher.go` - HTTP fetching with Browserless
- `internal/parser/parser.go` - HTML parsing
- `internal/storage/storage.go` - JSON storage and indexing
- `pkg/models/release.go` - Data models
