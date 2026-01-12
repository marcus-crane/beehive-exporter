package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcuswhybrow/beehive-exports/internal/fetcher"
	"github.com/marcuswhybrow/beehive-exports/internal/parser"
	"github.com/marcuswhybrow/beehive-exports/internal/storage"
	"github.com/marcuswhybrow/beehive-exports/pkg/models"
)

func main() {
	// Define commands
	syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
	syncPages := syncCmd.Int("pages", 0, "Max pagination pages to fetch (0 = all, ~10 releases per page)")

	reingestCmd := flag.NewFlagSet("reingest", flag.ExitOnError)
	reingestURL := reingestCmd.String("url", "", "Full URL of release to reingest")
	reingestID := reingestCmd.String("id", "", "ID/slug of release to reingest")

	markdownCmd := flag.NewFlagSet("markdown", flag.ExitOnError)
	markdownOut := markdownCmd.String("out", "content/markdown", "Output directory for markdown files")

	if len(os.Args) < 2 {
		fmt.Println("Usage: beehive-exports <command> [options]")
		fmt.Println("\nCommands:")
		fmt.Println("  sync       Fetch and save press releases")
		fmt.Println("  reingest   Re-fetch and overwrite a single release")
		fmt.Println("  markdown   Generate markdown files from releases")
		fmt.Println("\nExamples:")
		fmt.Println("  beehive-exports sync")
		fmt.Println("  beehive-exports sync --pages 3  # Fetch ~30 recent releases")
		fmt.Println("  beehive-exports reingest --url https://www.beehive.govt.nz/release/some-release")
		fmt.Println("  beehive-exports reingest --id some-release")
		fmt.Println("  beehive-exports markdown --out ./content/markdown")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "sync":
		syncCmd.Parse(os.Args[2:])
		runSync(*syncPages)
	case "reingest":
		reingestCmd.Parse(os.Args[2:])
		runReingest(*reingestURL, *reingestID)
	case "markdown":
		markdownCmd.Parse(os.Args[2:])
		runMarkdown(*markdownOut)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runSync(maxPages int) {
	log.Println("Starting sync")

	// Initialize components
	f := fetcher.New()
	store := storage.New(".")

	// Fetch release URLs
	urls, err := f.FetchReleases(maxPages)
	if err != nil {
		log.Fatalf("Failed to fetch release URLs: %v", err)
	}

	log.Printf("Found %d release URLs to process", len(urls))

	// Fetch and parse each release
	successCount := 0
	skippedCount := 0
	errorCount := 0

	for i, url := range urls {
		log.Printf("[%d/%d] Processing: %s", i+1, len(urls), url)

		// Parse to get ID (we'll do a quick parse just for the ID check)
		// For now, extract ID manually from URL
		parts := strings.Split(url, "/")
		id := parts[len(parts)-1]

		// Skip if already exists
		if store.ReleaseExists(id) {
			log.Printf("  Already exists, skipping")
			skippedCount++
			continue
		}

		// Fetch HTML
		html, err := f.FetchRelease(url)
		if err != nil {
			log.Printf("  Error fetching: %v", err)
			errorCount++
			continue
		}

		// Parse release
		release, err := parser.Parse(html, url)
		if err != nil {
			log.Printf("  Error parsing: %v", err)
			errorCount++
			continue
		}

		// Save to JSON
		if err := store.SaveRelease(release); err != nil {
			log.Printf("  Error saving: %v", err)
			errorCount++
			continue
		}

		log.Printf("  ✓ Saved: %s", release.Title)
		successCount++
	}

	// Update index
	log.Println("Updating index...")
	if err := store.UpdateIndex(); err != nil {
		log.Printf("Warning: Failed to update index: %v", err)
	}

	// Print summary
	log.Println("\n=== Sync Complete ===")
	log.Printf("Success: %d", successCount)
	log.Printf("Skipped: %d", skippedCount)
	log.Printf("Errors:  %d", errorCount)
	log.Printf("Total:   %d", len(urls))
}

func runReingest(url string, id string) {
	// Validate input
	if url == "" && id == "" {
		log.Fatal("Must provide either --url or --id")
	}

	// Build URL from ID if needed
	if url == "" {
		url = "https://www.beehive.govt.nz/release/" + id
	}

	log.Printf("Re-ingesting release: %s", url)

	// Initialize components
	f := fetcher.New()
	store := storage.New(".")

	// Fetch HTML
	html, err := f.FetchRelease(url)
	if err != nil {
		log.Fatalf("Error fetching: %v", err)
	}

	// Parse release
	release, err := parser.Parse(html, url)
	if err != nil {
		log.Fatalf("Error parsing: %v", err)
	}

	// Save to JSON (will overwrite if exists)
	if err := store.SaveRelease(release); err != nil {
		log.Fatalf("Error saving: %v", err)
	}

	log.Printf("✓ Re-ingested: %s", release.Title)
	log.Printf("  ID: %s", release.ID)
	log.Printf("  Time: %s", release.Time.Format(time.RFC3339))
	log.Printf("  Ministers: %d", len(release.Ministers))
	log.Printf("  Portfolios: %d", len(release.Portfolios))
	log.Printf("  Attachments: %d", len(release.Attachments))

	// Update index
	log.Println("Updating index...")
	if err := store.UpdateIndex(); err != nil {
		log.Printf("Warning: Failed to update index: %v", err)
	}

	log.Println("Done!")
}

func runMarkdown(outDir string) {
	log.Printf("Generating markdown files to %s", outDir)

	store := storage.New(".")
	releases, err := store.LoadAllReleases()
	if err != nil {
		log.Fatalf("Failed to load releases: %v", err)
	}

	log.Printf("Found %d releases", len(releases))

	for _, release := range releases {
		if err := writeMarkdown(outDir, release); err != nil {
			log.Printf("Error writing %s: %v", release.ID, err)
			continue
		}
	}

	log.Printf("Done! Generated %d markdown files", len(releases))
}

func writeMarkdown(outDir string, release *models.Release) error {
	year := release.Time.Format("2006")
	month := release.Time.Format("01")

	dir := filepath.Join(outDir, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var b strings.Builder

	// Front matter
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %q\n", release.Title)
	fmt.Fprintf(&b, "date: %s\n", release.Time.Format("2006-01-02"))
	fmt.Fprintf(&b, "url: %s\n", release.URL)
	if len(release.Ministers) > 0 {
		b.WriteString("ministers:\n")
		for _, m := range release.Ministers {
			fmt.Fprintf(&b, "  - %s\n", m.Name)
		}
	}
	if len(release.Portfolios) > 0 {
		b.WriteString("portfolios:\n")
		for _, p := range release.Portfolios {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}
	b.WriteString("---\n\n")

	// Content - extract body from full HTML and convert to markdown
	b.WriteString(htmlToMarkdown(extractBody(release.Content)))

	// Attachments
	if len(release.Attachments) > 0 {
		b.WriteString("\n\n## Attachments\n\n")
		for _, url := range release.Attachments {
			fmt.Fprintf(&b, "- %s\n", url)
		}
	}

	filename := filepath.Join(dir, release.ID+".md")
	return os.WriteFile(filename, []byte(b.String()), 0644)
}

func extractBody(html string) string {
	// Extract content from div.field--name-body
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	content := doc.Find("div.prose.field--name-body").First()
	if content.Length() == 0 {
		content = doc.Find("div.field--name-body").First()
	}

	if content.Length() == 0 {
		return ""
	}

	h, _ := content.Html()
	return h
}

func htmlToMarkdown(html string) string {
	s := html

	// Convert headers
	s = regexp.MustCompile(`<h1[^>]*>(.*?)</h1>`).ReplaceAllString(s, "# $1\n\n")
	s = regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`).ReplaceAllString(s, "## $1\n\n")
	s = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`).ReplaceAllString(s, "### $1\n\n")
	s = regexp.MustCompile(`<h4[^>]*>(.*?)</h4>`).ReplaceAllString(s, "#### $1\n\n")

	// Convert lists
	s = regexp.MustCompile(`<li[^>]*>(.*?)</li>`).ReplaceAllString(s, "- $1\n")
	s = regexp.MustCompile(`</?[uo]l[^>]*>`).ReplaceAllString(s, "\n")

	// Convert links
	s = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(s, "[$2]($1)")

	// Convert bold/italic
	s = regexp.MustCompile(`<strong[^>]*>(.*?)</strong>`).ReplaceAllString(s, "**$1**")
	s = regexp.MustCompile(`<b[^>]*>(.*?)</b>`).ReplaceAllString(s, "**$1**")
	s = regexp.MustCompile(`<em[^>]*>(.*?)</em>`).ReplaceAllString(s, "*$1*")
	s = regexp.MustCompile(`<i[^>]*>(.*?)</i>`).ReplaceAllString(s, "*$1*")

	// Convert paragraphs and breaks
	s = regexp.MustCompile(`<p[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`</p>`).ReplaceAllString(s, "\n\n")
	s = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(s, "\n")

	// Strip remaining tags
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")

	// Clean up whitespace
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)

	return s
}
