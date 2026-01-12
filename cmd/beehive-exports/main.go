package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/marcuswhybrow/beehive-exports/internal/fetcher"
	"github.com/marcuswhybrow/beehive-exports/internal/parser"
	"github.com/marcuswhybrow/beehive-exports/internal/storage"
)

func main() {
	// Define commands
	syncCmd := flag.NewFlagSet("sync", flag.ExitOnError)
	syncYear := syncCmd.Int("year", 2025, "Year to sync (default: 2025)")
	syncPages := syncCmd.Int("pages", 0, "Max pagination pages to fetch (0 = all, ~10 releases per page)")

	reingestCmd := flag.NewFlagSet("reingest", flag.ExitOnError)
	reingestURL := reingestCmd.String("url", "", "Full URL of release to reingest")
	reingestID := reingestCmd.String("id", "", "ID/slug of release to reingest")

	if len(os.Args) < 2 {
		fmt.Println("Usage: beehive-exports <command> [options]")
		fmt.Println("\nCommands:")
		fmt.Println("  sync       Fetch and save press releases")
		fmt.Println("  reingest   Re-fetch and overwrite a single release")
		fmt.Println("\nExamples:")
		fmt.Println("  beehive-exports sync --year 2025")
		fmt.Println("  beehive-exports sync --year 2025 --pages 3  # Fetch ~30 recent releases")
		fmt.Println("  beehive-exports reingest --url https://www.beehive.govt.nz/release/some-release")
		fmt.Println("  beehive-exports reingest --id some-release")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "sync":
		syncCmd.Parse(os.Args[2:])
		runSync(*syncYear, *syncPages)
	case "reingest":
		reingestCmd.Parse(os.Args[2:])
		runReingest(*reingestURL, *reingestID)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runSync(year int, maxPages int) {
	log.Printf("Starting sync for year %d", year)

	// Initialize components
	f := fetcher.New()
	store := storage.New(".")

	// Fetch release URLs
	urls, err := f.FetchReleasesByYear(year, maxPages)
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
	log.Printf("  Date: %s (%s)", release.Date, release.DateTime)
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
