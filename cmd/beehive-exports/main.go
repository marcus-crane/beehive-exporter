package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/marcus-crane/beehive-exports/internal/fetcher"
	"github.com/marcus-crane/beehive-exports/internal/parser"
	"github.com/marcus-crane/beehive-exports/internal/storage"
	"github.com/marcus-crane/beehive-exports/pkg/models"
)

func main() {
	// Load .env file if present
	_ = godotenv.Load()
	// Define commands
	syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
	syncPages := syncCmd.Int("pages", 0, "Max pagination pages to fetch (0 = all, ~10 releases per page)")

	reingestCmd := flag.NewFlagSet("reingest", flag.ExitOnError)
	reingestURL := reingestCmd.String("url", "", "Full URL of release to reingest")
	reingestID := reingestCmd.String("id", "", "ID/slug of release to reingest")

	markdownCmd := flag.NewFlagSet("markdown", flag.ExitOnError)
	markdownOut := markdownCmd.String("out", "content/markdown", "Output directory for markdown files")

	fetchCmd := flag.NewFlagSet("fetch", flag.ExitOnError)
	fetchYear := fetchCmd.String("year", "", "Year to fetch (e.g., 2024)")
	fetchMonth := fetchCmd.String("month", "", "Month to fetch (e.g., 2024-03)")

	if len(os.Args) < 2 {
		fmt.Println("Usage: beehive-exports <command> [options]")
		fmt.Println("\nCommands:")
		fmt.Println("  sync       Fetch and save press releases from listing pages")
		fmt.Println("  index      Build discovery index of all releases")
		fmt.Println("  fetch      Fetch releases from discovery index")
		fmt.Println("  reingest   Re-fetch and overwrite a single release")
		fmt.Println("  markdown   Generate markdown files from releases")
		fmt.Println("\nExamples:")
		fmt.Println("  beehive-exports sync")
		fmt.Println("  beehive-exports sync --pages 3  # Fetch ~30 recent releases")
		fmt.Println("  beehive-exports index           # Build complete release index")
		fmt.Println("  beehive-exports fetch           # Fetch all missing releases")
		fmt.Println("  beehive-exports fetch --year 2024")
		fmt.Println("  beehive-exports fetch --month 2024-03")
		fmt.Println("  beehive-exports reingest --url https://www.beehive.govt.nz/release/some-release")
		fmt.Println("  beehive-exports reingest --id some-release")
		fmt.Println("  beehive-exports markdown --out ./content/markdown")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "sync":
		syncCmd.Parse(os.Args[2:])
		runSync(*syncPages)
	case "index":
		runIndex()
	case "fetch":
		fetchCmd.Parse(os.Args[2:])
		runFetch(*fetchYear, *fetchMonth)
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

		// Save raw HTML for potential future reprocessing
		if err := store.SaveRawHTML(release.ID, release.Time, html); err != nil {
			log.Printf("  Warning: Failed to save raw HTML: %v", err)
		}

		log.Printf("  ✓ Saved: %s", release.Title)
		successCount++
	}

	// Update index
	log.Println("Updating index...")
	if err := store.UpdateIndex(); err != nil {
		log.Printf("Warning: Failed to update index: %v", err)
	}

	// Update URL index
	log.Println("Updating URL index...")
	if err := store.GenerateURLIndex(); err != nil {
		log.Printf("Warning: Failed to update URL index: %v", err)
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

	// Save raw HTML for potential future reprocessing
	if err := store.SaveRawHTML(release.ID, release.Time, html); err != nil {
		log.Printf("Warning: Failed to save raw HTML: %v", err)
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
	if len(release.Attachments) > 0 {
		b.WriteString("attachments:\n")
		for _, url := range release.Attachments {
			fmt.Fprintf(&b, "  - %s\n", url)
		}
	}
	b.WriteString("---\n\n")

	// Content - extract body from full HTML and convert to markdown
	b.WriteString(htmlToMarkdown(extractBody(release.Content)))

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

func htmlToMarkdown(input string) string {
	md, err := htmltomarkdown.ConvertString(input)
	if err != nil {
		return input
	}
	return strings.TrimSpace(md)
}

// ReleaseEntry represents a discovered release with its date
type ReleaseEntry struct {
	URL  string `json:"url"`
	Date string `json:"date"`
}

// DiscoveryIndex is the structure for the discovered releases index
type DiscoveryIndex struct {
	LastIndexedAt string                                   `json:"last_indexed_at"`
	TotalReleases int                                      `json:"total_releases"`
	Governments   map[string]map[string][]ReleaseEntry     `json:"governments"` // government -> year-month -> releases
}

func runIndex() {
	log.Println("Building complete release index from listing pages...")

	f := fetcher.New()
	index := DiscoveryIndex{
		Governments: make(map[string]map[string][]ReleaseEntry),
	}

	page := 0
	consecutiveEmpty := 0
	totalFound := 0

	for consecutiveEmpty < 3 {
		log.Printf("Fetching page %d...", page)

		url := fmt.Sprintf("https://www.beehive.govt.nz/releases?page=%d", page)
		html, err := f.FetchURL(url)
		if err != nil {
			log.Printf("Error fetching page %d: %v", page, err)
			consecutiveEmpty++
			page++
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			log.Printf("Error parsing page %d: %v", page, err)
			consecutiveEmpty++
			page++
			continue
		}

		foundOnPage := 0
		seen := make(map[string]bool)

		// Find each release entry (article or container with time and link)
		doc.Find("a[href^='/release/']").Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if !exists || href == "/release" || href == "/releases" {
				return
			}

			fullURL := "https://www.beehive.govt.nz" + href
			if seen[fullURL] {
				return
			}
			seen[fullURL] = true

			// Find the time element in the parent/grandparent context
			var datetime string
			parent := s.Parent().Parent().Parent()
			parent.Find("time[datetime]").Each(func(j int, t *goquery.Selection) {
				if dt, exists := t.Attr("datetime"); exists && datetime == "" {
					datetime = dt
				}
			})

			if datetime == "" {
				return // Skip if no date found
			}

			// Parse date to determine government and month
			t, err := time.Parse(time.RFC3339, datetime)
			if err != nil {
				return
			}

			gov := determineGovernmentFromDate(t)
			yearMonth := t.Format("2006-01")

			if index.Governments[gov] == nil {
				index.Governments[gov] = make(map[string][]ReleaseEntry)
			}

			index.Governments[gov][yearMonth] = append(index.Governments[gov][yearMonth], ReleaseEntry{
				URL:  fullURL,
				Date: datetime,
			})

			foundOnPage++
			totalFound++
		})

		log.Printf("Found %d releases on page %d", foundOnPage, page)

		if foundOnPage == 0 {
			consecutiveEmpty++
		} else {
			consecutiveEmpty = 0
		}

		page++
	}

	index.LastIndexedAt = time.Now().Format(time.RFC3339)
	index.TotalReleases = totalFound

	// Save index
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal index: %v", err)
	}

	if err := os.WriteFile("content/discovery-index.json", data, 0644); err != nil {
		log.Fatalf("Failed to write index: %v", err)
	}

	log.Printf("\n=== Indexing Complete ===")
	log.Printf("Total releases found: %d", totalFound)
	log.Printf("Pages scanned: %d", page)
	log.Printf("Index saved to content/discovery-index.json")

	// Print summary by government
	for gov, months := range index.Governments {
		count := 0
		for _, releases := range months {
			count += len(releases)
		}
		log.Printf("  %s: %d releases", gov, count)
	}
}

func runFetch(filterYear, filterMonth string) {
	log.Println("Fetching releases from discovery index")

	// Load discovery index
	data, err := os.ReadFile("content/discovery-index.json")
	if err != nil {
		log.Fatalf("Failed to read discovery index: %v\nRun 'beehive-exports index' first", err)
	}

	var index DiscoveryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		log.Fatalf("Failed to parse discovery index: %v", err)
	}

	// Collect URLs to fetch based on filters
	var urlsToFetch []string
	for _, months := range index.Governments {
		for yearMonth, releases := range months {
			// Apply filters
			if filterMonth != "" && yearMonth != filterMonth {
				continue
			}
			if filterYear != "" && !strings.HasPrefix(yearMonth, filterYear) {
				continue
			}

			for _, r := range releases {
				urlsToFetch = append(urlsToFetch, r.URL)
			}
		}
	}

	log.Printf("Found %d URLs in index matching filters", len(urlsToFetch))

	// Initialize components
	f := fetcher.New()
	store := storage.New(".")

	// Fetch and parse each release
	successCount := 0
	skippedCount := 0
	errorCount := 0

	for i, url := range urlsToFetch {
		// Extract ID from URL
		parts := strings.Split(url, "/")
		id := parts[len(parts)-1]

		// Skip if already exists
		if store.ReleaseExists(id) {
			skippedCount++
			continue
		}

		log.Printf("[%d/%d] Fetching: %s", i+1, len(urlsToFetch), url)

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

		// Save raw HTML for potential future reprocessing
		if err := store.SaveRawHTML(release.ID, release.Time, html); err != nil {
			log.Printf("  Warning: Failed to save raw HTML: %v", err)
		}

		log.Printf("  ✓ Saved: %s", release.Title)
		successCount++
	}

	// Update index
	log.Println("Updating index...")
	if err := store.UpdateIndex(); err != nil {
		log.Printf("Warning: Failed to update index: %v", err)
	}

	// Update URL index
	log.Println("Updating URL index...")
	if err := store.GenerateURLIndex(); err != nil {
		log.Printf("Warning: Failed to update URL index: %v", err)
	}

	// Print summary
	log.Println("\n=== Fetch Complete ===")
	log.Printf("Success: %d", successCount)
	log.Printf("Skipped: %d (already exist)", skippedCount)
	log.Printf("Errors:  %d", errorCount)
	log.Printf("Total:   %d", len(urlsToFetch))
}

func determineGovernmentFromDate(t time.Time) string {
	year := t.Year()
	month := t.Month()

	// More precise government terms based on election dates
	switch {
	case year > 2023 || (year == 2023 && month >= 11):
		return "National-ACT-NZ First Coalition"
	case year > 2020 || (year == 2020 && month >= 11):
		return "Sixth Labour Government"
	case year > 2017 || (year == 2017 && month >= 10):
		return "Labour-NZ First Coalition"
	case year > 2014 || (year == 2014 && month >= 10):
		return "Fifth National Government"
	case year > 2011 || (year == 2011 && month >= 11):
		return "Fifth National Government"
	case year > 2008 || (year == 2008 && month >= 11):
		return "Fifth National Government"
	default:
		return "Earlier Government"
	}
}

