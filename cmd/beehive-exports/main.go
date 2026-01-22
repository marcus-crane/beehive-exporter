package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	reingestCmd := flag.NewFlagSet("reingest", flag.ExitOnError)
	reingestURL := reingestCmd.String("url", "", "Full URL of release to reingest")
	reingestID := reingestCmd.String("id", "", "ID/slug of release to reingest")
	reingestCmd.Usage = func() {
		fmt.Println("Usage: beehive-exports reingest [options]")
		fmt.Println("\nOptions:")
		fmt.Println("  --url string")
		fmt.Println("        Full URL of release to reingest")
		fmt.Println("  --id string")
		fmt.Println("        ID/slug of release to reingest")
		fmt.Println("\nProvide either --url or --id, not both.")
	}

	fetchCmd := flag.NewFlagSet("fetch", flag.ExitOnError)
	fetchYear := fetchCmd.String("year", "", "Year to fetch (e.g., 2024)")
	fetchMonth := fetchCmd.String("month", "", "Month to fetch (e.g., 2024-03)")
	fetchGov := fetchCmd.String("government", "", "Government to fetch (e.g., national-1993)")
	fetchVerbose := fetchCmd.Bool("verbose", false, "Log skipped posts that already exist")
	fetchCmd.Usage = func() {
		fmt.Println("Usage: beehive-exports fetch [options]")
		fmt.Println("\nOptions:")
		fmt.Println("  --year string")
		fmt.Println("        Year to fetch (e.g., 2024)")
		fmt.Println("  --month string")
		fmt.Println("        Month to fetch (e.g., 2024-03)")
		fmt.Println("  --government string")
		fmt.Println("        Government to fetch. If not specified, fetches all.")
		fmt.Println("  --verbose")
		fmt.Println("        Log skipped posts that already exist")
		fmt.Println("\nValid government values:")
		fmt.Println("  1993-1996-national   National")
		fmt.Println("  1996-1999-national   National-NZ First Coalition")
		fmt.Println("  1999-2002-labour     Labour-Alliance")
		fmt.Println("  2002-2005-labour     Labour-Progressive Coalition")
		fmt.Println("  2005-2008-labour     Labour-Progressive Coalition")
		fmt.Println("  2008-2011-national   Fifth National Government")
		fmt.Println("  2011-2014-national   Fifth National Government")
		fmt.Println("  2014-2017-national   Fifth National Government")
		fmt.Println("  2017-2020-labour     Labour-NZ First Coalition")
		fmt.Println("  2020-2023-labour     Sixth Labour Government")
		fmt.Println("  2023-2026-national   National-ACT-NZ First Coalition")
	}

	processCmd := flag.NewFlagSet("process", flag.ExitOnError)
	processJSON := processCmd.Bool("json", false, "Generate only JSON files")
	processMD := processCmd.Bool("markdown", false, "Generate only Markdown files")
	processCmd.Usage = func() {
		fmt.Println("Usage: beehive-exports process [options]")
		fmt.Println("\nOptions:")
		fmt.Println("  --json")
		fmt.Println("        Generate only JSON files")
		fmt.Println("  --markdown")
		fmt.Println("        Generate only Markdown files")
		fmt.Println("\nIf no flags specified, generates both JSON and Markdown.")
	}

	archiveCmd := flag.NewFlagSet("archive", flag.ExitOnError)
	archiveGov := archiveCmd.String("government", "", "Government term to index (see --help for list)")
	archivePages := archiveCmd.Int("pages", 0, "Maximum pages to index (0 = all)")
	archiveStart := archiveCmd.Int("start", 0, "Starting page number (0-indexed)")
	archiveType := archiveCmd.String("type", "release", "Content type to index (release, speech, feature, diary)")
	archiveCmd.Usage = func() {
		fmt.Println("Usage: beehive-exports archive [options]")
		fmt.Println("\nOptions:")
		fmt.Println("  --government string")
		fmt.Println("        Government term to index. If not specified, indexes all governments.")
		fmt.Println("  --pages int")
		fmt.Println("        Maximum pages to index, 24 results per page (0 = all, default 0)")
		fmt.Println("  --start int")
		fmt.Println("        Starting page number, 0-indexed (default 0)")
		fmt.Println("  --type string")
		fmt.Println("        Content type to index (default \"release\")")
		fmt.Println("\nValid government values:")
		fmt.Println("  1993-1996-national   National")
		fmt.Println("  1996-1999-national   National-NZ First Coalition")
		fmt.Println("  1999-2002-labour     Labour-Alliance")
		fmt.Println("  2002-2005-labour     Labour-Progressive Coalition")
		fmt.Println("  2005-2008-labour     Labour-Progressive Coalition")
		fmt.Println("  2008-2011-national   Fifth National Government")
		fmt.Println("  2011-2014-national   Fifth National Government")
		fmt.Println("  2014-2017-national   Fifth National Government")
		fmt.Println("  2017-2020-labour     Labour-NZ First Coalition")
		fmt.Println("  2020-2023-labour     Sixth Labour Government")
		fmt.Println("  2023-2026-national   National-ACT-NZ First Coalition")
		fmt.Println("\nValid content types:")
		fmt.Println("  release   Press releases (54,966)")
		fmt.Println("  speech    Speeches (12,355)")
		fmt.Println("  feature   Features (1,279)")
		fmt.Println("  diary     Ministerial diaries (2,347)")
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: beehive-exports <command> [options]")
		fmt.Println("\nCommands:")
		fmt.Println("  archive    Build discovery index from search page")
		fmt.Println("  fetch      Fetch raw HTML from discovery index")
		fmt.Println("  process    Generate JSON + Markdown from raw HTML")
		fmt.Println("  reingest   Re-fetch and overwrite a single release")
		fmt.Println("\nExamples:")
		fmt.Println("  beehive-exports archive                                  # Index all releases")
		fmt.Println("  beehive-exports archive --government 2023-2026-national  # Index one government")
		fmt.Println("  beehive-exports archive --pages 5                        # Index first 5 pages (120 items)")
		fmt.Println("  beehive-exports archive --type speech                    # Index speeches instead of releases")
		fmt.Println("  beehive-exports archive --help                           # Show all options")
		fmt.Println("  beehive-exports fetch                                    # Fetch all indexed releases")
		fmt.Println("  beehive-exports fetch --government 1993-1996-national    # Fetch one government")
		fmt.Println("  beehive-exports fetch --year 2024                        # Fetch by year")
		fmt.Println("  beehive-exports process                                  # Generate JSON + Markdown from raw HTML")
		fmt.Println("  beehive-exports reingest --id some-release               # Re-fetch single release")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "fetch":
		fetchCmd.Parse(os.Args[2:])
		runFetch(*fetchYear, *fetchMonth, *fetchGov, *fetchVerbose)
	case "process":
		processCmd.Parse(os.Args[2:])
		runProcess(*processJSON, *processMD)
	case "reingest":
		reingestCmd.Parse(os.Args[2:])
		runReingest(*reingestURL, *reingestID)
	case "archive":
		archiveCmd.Parse(os.Args[2:])
		runArchive(*archiveGov, *archivePages, *archiveStart, *archiveType)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
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
	release, err := parser.Parse(html, url, time.Now())
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

func runProcess(jsonOnly, markdownOnly bool) {
	// Default to both if neither flag is set
	doJSON := !markdownOnly || jsonOnly
	doMarkdown := !jsonOnly || markdownOnly
	if !jsonOnly && !markdownOnly {
		doJSON = true
		doMarkdown = true
	}

	log.Println("Processing raw HTML files...")
	if doJSON && !doMarkdown {
		log.Println("  (JSON only)")
	} else if doMarkdown && !doJSON {
		log.Println("  (Markdown only)")
	}

	store := storage.New(".")
	log.Println("Scanning for raw HTML files...")
	files, err := store.LoadAllRawHTML()
	if err != nil {
		log.Fatalf("Failed to load raw HTML files: %v", err)
	}

	log.Printf("Found %d raw HTML files", len(files))

	successCount := 0
	errorCount := 0
	markdownDir := "content/markdown"

	for i, file := range files {
		// Get file modification time (when it was scraped)
		fileInfo, err := os.Stat(file.Path)
		if err != nil {
			log.Printf("Error getting file info %s: %v", file.Path, err)
			errorCount++
			continue
		}
		scrapedAt := fileInfo.ModTime()

		html, err := store.ReadRawHTML(file.Path)
		if err != nil {
			log.Printf("Error reading %s: %v", file.Path, err)
			errorCount++
			continue
		}

		// Reconstruct URL from file path (content/raw/YYYY/MM/YYYY-MM-DD-id.html)
		url := "https://www.beehive.govt.nz/release/" + file.ID

		release, err := parser.Parse(html, url, scrapedAt)
		if err != nil {
			log.Printf("Error parsing %s: %v", file.Path, err)
			errorCount++
			continue
		}

		// Save JSON
		if doJSON {
			if err := store.SaveRelease(release); err != nil {
				log.Printf("Error saving JSON for %s: %v", file.ID, err)
				errorCount++
				continue
			}
		}

		// Save Markdown
		if doMarkdown {
			if err := writeMarkdown(markdownDir, release); err != nil {
				log.Printf("Error saving Markdown for %s: %v", file.ID, err)
				errorCount++
				continue
			}
		}

		if (i+1)%100 == 0 {
			log.Printf("  Processed %d/%d files...", i+1, len(files))
		}
		successCount++
	}

	// Update indexes (only if we generated JSON)
	if doJSON {
		log.Println("Updating index...")
		if err := store.UpdateIndex(); err != nil {
			log.Printf("Warning: Failed to update index: %v", err)
		}

		log.Println("Updating URL index...")
		if err := store.GenerateURLIndex(); err != nil {
			log.Printf("Warning: Failed to update URL index: %v", err)
		}
	}

	log.Println("\n=== Process Complete ===")
	log.Printf("Success: %d", successCount)
	log.Printf("Errors:  %d", errorCount)
	log.Printf("Total:   %d", len(files))
}

func writeMarkdown(outDir string, release *models.Release) error {
	year := release.Time.Format("2006")
	month := release.Time.Format("01")
	day := release.Time.Format("2006-01-02")

	dir := filepath.Join(outDir, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Filename with date prefix: YYYY-MM-DD-id.md
	filename := filepath.Join(dir, day+"-"+release.ID+".md")

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

// GovernmentIndex is the structure for a single government's release index
type GovernmentIndex struct {
	Government    string                    `json:"government"`
	LastIndexedAt string                    `json:"last_indexed_at"`
	TotalReleases int                       `json:"total_releases"`
	Releases      map[string][]ReleaseEntry `json:"releases"` // year-month -> releases
}

// DiscoveryIndex is the structure for the discovered releases index (legacy, kept for migration)
type DiscoveryIndex struct {
	LastIndexedAt string                                   `json:"last_indexed_at"`
	TotalReleases int                                      `json:"total_releases"`
	Governments   map[string]map[string][]ReleaseEntry     `json:"governments"` // government -> year-month -> releases
}

const discoveryIndexDir = "content/discovery-index"

// saveGovernmentIndex saves a single government's index to its own file
func saveGovernmentIndex(slug string, idx *GovernmentIndex) error {
	if err := os.MkdirAll(discoveryIndexDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Sort releases within each month by date (oldest first), then by URL
	for month := range idx.Releases {
		releases := idx.Releases[month]
		sort.Slice(releases, func(i, j int) bool {
			if releases[i].Date != releases[j].Date {
				return releases[i].Date < releases[j].Date
			}
			return releases[i].URL < releases[j].URL
		})
		idx.Releases[month] = releases
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	filename := filepath.Join(discoveryIndexDir, slug+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// loadGovernmentIndex loads a single government's index
func loadGovernmentIndex(slug string) (*GovernmentIndex, error) {
	filename := filepath.Join(discoveryIndexDir, slug+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var idx GovernmentIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	return &idx, nil
}

// loadAllGovernmentIndexes loads all government indexes from the discovery-index directory
func loadAllGovernmentIndexes() ([]*GovernmentIndex, error) {
	entries, err := os.ReadDir(discoveryIndexDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var indexes []*GovernmentIndex
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		slug := strings.TrimSuffix(entry.Name(), ".json")
		idx, err := loadGovernmentIndex(slug)
		if err != nil {
			log.Printf("Warning: failed to load %s: %v", entry.Name(), err)
			continue
		}
		indexes = append(indexes, idx)
	}

	return indexes, nil
}

func runFetch(filterYear, filterMonth, filterGov string, verbose bool) {
	log.Println("Fetching releases from discovery index")

	// Load government indexes
	var indexes []*GovernmentIndex

	if filterGov != "" {
		// Load specific government
		idx, err := loadGovernmentIndex(filterGov)
		if err != nil {
			log.Fatalf("Failed to load government index %s: %v\nRun 'beehive-exports archive --government %s' first", filterGov, err, filterGov)
		}
		indexes = append(indexes, idx)
	} else {
		// Load all government indexes
		var err error
		indexes, err = loadAllGovernmentIndexes()
		if err != nil {
			log.Fatalf("Failed to load discovery indexes: %v", err)
		}
		if len(indexes) == 0 {
			log.Fatalf("No discovery indexes found in %s/\nRun 'beehive-exports archive' or 'beehive-exports index' first", discoveryIndexDir)
		}
	}

	// Collect releases to fetch based on filters
	var releasesToFetch []ReleaseEntry
	for _, idx := range indexes {
		for yearMonth, releases := range idx.Releases {
			// Apply filters
			if filterMonth != "" && yearMonth != filterMonth {
				continue
			}
			if filterYear != "" && !strings.HasPrefix(yearMonth, filterYear) {
				continue
			}

			releasesToFetch = append(releasesToFetch, releases...)
		}
	}

	// Initialize components
	f := fetcher.New()
	store := storage.New(".")

	// Count how many already exist using direct path lookup
	log.Printf("Checking %d releases for existing downloads...", len(releasesToFetch))
	existingCount := 0
	for i, r := range releasesToFetch {
		id := parser.ExtractIDFromURL(r.URL)
		releaseDate, err := time.Parse(time.RFC3339, r.Date)
		if err != nil {
			// Fall back to parsing just the date portion
			releaseDate, _ = time.Parse("2006-01-02", r.Date[:10])
		}
		if store.RawHTMLExistsWithDate(id, releaseDate) {
			existingCount++
		}
		if (i+1)%1000 == 0 {
			log.Printf("  Checked %d/%d releases (%d exist)...", i+1, len(releasesToFetch), existingCount)
		}
	}

	remaining := len(releasesToFetch) - existingCount
	log.Printf("Found %d releases in index (%d already downloaded, %d remaining)", len(releasesToFetch), existingCount, remaining)

	if remaining == 0 {
		log.Println("Nothing to fetch - all releases already downloaded")
		return
	}

	// Fetch and parse each release
	successCount := 0
	errorCount := 0
	fetchCount := 0

	for _, r := range releasesToFetch {
		// Extract and normalize ID from URL
		id := parser.ExtractIDFromURL(r.URL)

		// Skip if raw HTML already exists
		releaseDate, err := time.Parse(time.RFC3339, r.Date)
		if err != nil {
			releaseDate, _ = time.Parse("2006-01-02", r.Date[:10])
		}
		if store.RawHTMLExistsWithDate(id, releaseDate) {
			if verbose {
				log.Printf("Skipping (already exists): %s", r.URL)
			}
			continue
		}

		fetchCount++
		log.Printf("[%d/%d] Fetching: %s", fetchCount, remaining, r.URL)

		// Fetch HTML
		html, err := f.FetchRelease(r.URL)
		if err != nil {
			log.Printf("  Error fetching: %v", err)
			errorCount++
			continue
		}

		// Parse just to get ID and time for file naming
		release, err := parser.Parse(html, r.URL, time.Now())
		if err != nil {
			log.Printf("  Error parsing: %v", err)
			errorCount++
			continue
		}

		// Save raw HTML only
		if err := store.SaveRawHTML(release.ID, release.Time, html); err != nil {
			log.Printf("  Error saving: %v", err)
			errorCount++
			continue
		}

		log.Printf("  ✓ Saved: %s", release.Title)
		successCount++
	}

	// Print summary
	log.Println("\n=== Fetch Complete ===")
	log.Printf("Downloaded: %d", successCount)
	log.Printf("Errors:     %d", errorCount)
	log.Printf("Skipped:    %d (already existed)", existingCount)
	log.Println("\nRun 'beehive-exports process' to generate JSON and Markdown files")
}

// GovernmentFacet maps CLI-friendly names to beehive.govt.nz facet IDs
var governmentFacets = map[string]struct {
	FacetID string
	Name    string
}{
	"2023-2026-national": {"6700", "National-ACT-NZ First Coalition"},
	"2020-2023-labour":   {"6455", "Sixth Labour Government"},
	"2017-2020-labour":   {"6203", "Labour-NZ First Coalition"},
	"2014-2017-national": {"6064", "Fifth National Government"},
	"2011-2014-national": {"5926", "Fifth National Government"},
	"2008-2011-national": {"4775", "Fifth National Government"},
	"2005-2008-labour":   {"4637", "Labour-Progressive Coalition"},
	"2002-2005-labour":   {"4507", "Labour-Progressive Coalition"},
	"1999-2002-labour":   {"4376", "Labour-Alliance"},
	"1996-1999-national": {"4265", "National-NZ First Coalition"},
	"1993-1996-national": {"4194", "National"},
}

// contentTypeFacets maps CLI-friendly names to beehive.govt.nz facet values and URL prefixes
var contentTypeFacets = map[string]struct {
	Facet     string
	URLPrefix string
}{
	"release": {"article", "/release/"},
	"speech":  {"speech", "/speech/"},
	"feature": {"feature", "/feature/"},
	"diary":   {"ministerial_diary", "/ministerial-diary/"},
}

func runArchive(govFilter string, maxPages int, startPage int, contentType string) {
	// Validate content type
	ct, ok := contentTypeFacets[contentType]
	if !ok {
		log.Fatalf("Unknown content type: %s\nValid types: release, speech, feature, diary", contentType)
	}

	// Check for browserless token - search pages require JavaScript to render results
	if os.Getenv("BROWSERLESS_TOKEN") == "" {
		log.Println("WARNING: BROWSERLESS_TOKEN not set. Search pages require JavaScript to render results.")
		log.Println("         Results may be incomplete. Set BROWSERLESS_TOKEN for full discovery.")
	}

	log.Printf("Building discovery index from search page (type: %s)...", contentType)

	// Determine which governments to index
	var toIndex []struct {
		Key     string
		FacetID string
		Name    string
	}

	if govFilter != "" {
		gov, ok := governmentFacets[govFilter]
		if !ok {
			log.Fatalf("Unknown government: %s\nRun 'beehive-exports archive --help' for valid options", govFilter)
		}
		toIndex = append(toIndex, struct {
			Key     string
			FacetID string
			Name    string
		}{govFilter, gov.FacetID, gov.Name})
	} else {
		for k, v := range governmentFacets {
			toIndex = append(toIndex, struct {
				Key     string
				FacetID string
				Name    string
			}{k, v.FacetID, v.Name})
		}
		// Sort by year descending (newest first)
		sort.Slice(toIndex, func(i, j int) bool {
			// Keys are like "2023-2026-national" - extract start year from beginning
			yi := toIndex[i].Key[:4]
			yj := toIndex[j].Key[:4]
			return yi > yj
		})
	}

	f := fetcher.New()
	totalFound := 0
	globalPageCount := 0
	globalPageLimit := govFilter == "" && maxPages > 0 // Apply global limit only when no gov filter

	for _, gov := range toIndex {
		// Check global page limit before starting new government
		if globalPageLimit && globalPageCount >= maxPages {
			log.Printf("\nReached global page limit (%d)", maxPages)
			break
		}

		log.Printf("\nIndexing %s (facet: %s)...", gov.Name, gov.FacetID)

		// Load existing index or create new one
		govIndex, err := loadGovernmentIndex(gov.Key)
		if err != nil {
			// No existing index, create new one
			govIndex = &GovernmentIndex{
				Government: gov.Name,
				Releases:   make(map[string][]ReleaseEntry),
			}
		}

		// Build set of existing URLs for deduplication
		existingURLs := make(map[string]bool)
		for _, entries := range govIndex.Releases {
			for _, entry := range entries {
				existingURLs[entry.URL] = true
			}
		}

		page := startPage
		govTotal := 0
		consecutiveEmpty := 0

		for {
			// Check page limit (global if no gov filter, per-gov otherwise)
			if globalPageLimit && globalPageCount >= maxPages {
				log.Printf("  Reached global page limit (%d)", maxPages)
				break
			}
			if !globalPageLimit && maxPages > 0 && page >= startPage+maxPages {
				log.Printf("  Reached page limit (%d pages from start)", maxPages)
				break
			}

			url := fmt.Sprintf("https://www.beehive.govt.nz/search?f[0]=government_facet:%s&f[1]=content_type_facet:%s&page=%d", gov.FacetID, ct.Facet, page)
			log.Printf("  Fetching page %d (global: %d)...", page, globalPageCount)

			html, err := f.FetchURL(url)
			if err != nil {
				log.Printf("  Error fetching page %d: %v", page, err)
				break
			}

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
			if err != nil {
				log.Printf("  Error parsing page %d: %v", page, err)
				break
			}

			foundOnPage := 0

			// Find links matching the content type's URL prefix
			selector := fmt.Sprintf("a[href^='%s']", ct.URLPrefix)
			doc.Find(selector).Each(func(i int, s *goquery.Selection) {
				href, exists := s.Attr("href")
				if !exists || href == ct.URLPrefix || href == strings.TrimSuffix(ct.URLPrefix, "/") {
					return
				}

				fullURL := "https://www.beehive.govt.nz" + href

				// Find date - look for time element in the same search result container
				var datetime string
				// Walk up to find the containing article/div then look for time
				container := s.Closest("article, .views-row, .search-result")
				if container.Length() > 0 {
					container.Find("time[datetime]").Each(func(j int, t *goquery.Selection) {
						if dt, exists := t.Attr("datetime"); exists && datetime == "" {
							datetime = dt
						}
					})
				}

				// Fallback: check siblings and parent structures
				if datetime == "" {
					s.Parent().Parent().Find("time[datetime]").Each(func(j int, t *goquery.Selection) {
						if dt, exists := t.Attr("datetime"); exists && datetime == "" {
							datetime = dt
						}
					})
				}

				if datetime == "" {
					// Use a placeholder date if none found - we can fix later
					datetime = "0001-01-01T00:00:00Z"
				}

				// Parse date to get year-month
				t, err := time.Parse(time.RFC3339, datetime)
				if err != nil {
					// Try alternate format
					t, err = time.Parse("2006-01-02", datetime[:10])
					if err != nil {
						t = time.Time{}
					}
				}

				yearMonth := t.Format("2006-01")
				foundOnPage++

				// Skip if already indexed
				if existingURLs[fullURL] {
					return
				}
				existingURLs[fullURL] = true

				govIndex.Releases[yearMonth] = append(govIndex.Releases[yearMonth], ReleaseEntry{
					URL:  fullURL,
					Date: datetime,
				})

				govTotal++
			})

			// Also find /node/ links for historical releases (pre-2014 content)
			// These redirect to actual release URLs when fetched
			if ct.URLPrefix == "/release/" {
				doc.Find("a[href^='/node/']").Each(func(i int, s *goquery.Selection) {
					href, exists := s.Attr("href")
					if !exists || href == "/node/" {
						return
					}

					fullURL := "https://www.beehive.govt.nz" + href
					foundOnPage++ // Count ALL items found, not just new ones

					// Skip if already indexed
					if existingURLs[fullURL] {
						return
					}

					// Find date - look for time element in the same search result container
					var datetime string
					container := s.Closest("article, .views-row, .search-result")
					if container.Length() > 0 {
						container.Find("time[datetime]").Each(func(j int, t *goquery.Selection) {
							if dt, exists := t.Attr("datetime"); exists && datetime == "" {
								datetime = dt
							}
						})
					}

					// Fallback: check siblings and parent structures
					if datetime == "" {
						s.Parent().Parent().Find("time[datetime]").Each(func(j int, t *goquery.Selection) {
							if dt, exists := t.Attr("datetime"); exists && datetime == "" {
								datetime = dt
							}
						})
					}

					if datetime == "" {
						datetime = "0001-01-01T00:00:00Z"
					}

					// Parse date to get year-month
					t, err := time.Parse(time.RFC3339, datetime)
					if err != nil {
						t, err = time.Parse("2006-01-02", datetime[:10])
						if err != nil {
							t = time.Time{}
						}
					}

					yearMonth := t.Format("2006-01")
					existingURLs[fullURL] = true

					govIndex.Releases[yearMonth] = append(govIndex.Releases[yearMonth], ReleaseEntry{
						URL:  fullURL,
						Date: datetime,
					})

					govTotal++
				})
			}

			log.Printf("  Found %d items on page %d (%d new so far)", foundOnPage, page, govTotal)

			// Save incrementally after each page with new items
			if govTotal > 0 {
				govIndex.LastIndexedAt = time.Now().Format(time.RFC3339)
				totalReleases := 0
				for _, entries := range govIndex.Releases {
					totalReleases += len(entries)
				}
				govIndex.TotalReleases = totalReleases

				if err := saveGovernmentIndex(gov.Key, govIndex); err != nil {
					log.Printf("  Error saving index: %v", err)
				}
			}

			if foundOnPage == 0 {
				consecutiveEmpty++
				if consecutiveEmpty >= 3 {
					log.Printf("  Stopping after %d consecutive empty pages", consecutiveEmpty)
					break
				}
				log.Printf("  Empty page, continuing (%d consecutive)...", consecutiveEmpty)
			} else {
				consecutiveEmpty = 0
			}

			page++
			globalPageCount++
		}

		// Final save and summary
		totalReleases := 0
		for _, entries := range govIndex.Releases {
			totalReleases += len(entries)
		}
		log.Printf("  Completed %s (%d new, %d total)", gov.Name, govTotal, totalReleases)

		totalFound += govTotal
	}

	log.Printf("\n=== Archive Indexing Complete ===")
	log.Printf("Releases indexed this run: %d", totalFound)
	log.Printf("Index files saved to %s/", discoveryIndexDir)
}

