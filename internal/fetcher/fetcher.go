package fetcher

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

const (
	BaseURL        = "https://www.beehive.govt.nz"
	RSSFeedURL     = BaseURL + "/releases/feed"
	ReleasesURL    = BaseURL + "/releases"
	RateLimitDelay = 1000 * time.Millisecond // 1 second between requests

	BrowserlessURL = "https://browser.home.utf9k.net"
)

// Fetcher handles HTTP requests with rate limiting
type Fetcher struct {
	client           *http.Client
	lastRequestAt    time.Time
	browserlessToken string
}

// New creates a new Fetcher instance
func New() *Fetcher {
	token := os.Getenv("BROWSERLESS_TOKEN")
	if token != "" {
		log.Println("Browserless token found, will use headless browser for fetching")
	}
	return &Fetcher{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		browserlessToken: token,
	}
}

// rateLimit waits if necessary to maintain rate limiting
func (f *Fetcher) rateLimit() {
	elapsed := time.Since(f.lastRequestAt)
	if elapsed < RateLimitDelay {
		time.Sleep(RateLimitDelay - elapsed)
	}
	f.lastRequestAt = time.Now()
}

// doRequest performs a single HTTP request and returns the body
func (f *Fetcher) doRequest(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers to look like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Handle gzip compression if present
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer gzReader.Close()
		reader = gzReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// doBrowserlessRequest fetches a URL using the Browserless headless browser
func (f *Fetcher) doBrowserlessRequest(targetURL string) (string, error) {
	endpoint := fmt.Sprintf("%s/content?token=%s", BrowserlessURL, f.browserlessToken)

	payload := map[string]interface{}{
		"url":          targetURL,
		"waitForTimeout": 3000,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("browserless returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// FetchURL fetches a URL with rate limiting and retry logic
func (f *Fetcher) FetchURL(url string) (string, error) {
	f.rateLimit()

	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		var body string
		var err error

		if f.browserlessToken != "" {
			body, err = f.doBrowserlessRequest(url)
		} else {
			body, err = f.doRequest(url)
		}

		if err == nil {
			return body, nil
		}
		lastErr = err
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
		}
	}

	return "", fmt.Errorf("failed to fetch %s after %d attempts: %w", url, maxRetries, lastErr)
}

// FetchFromRSS fetches recent release URLs from the RSS feed
func (f *Fetcher) FetchFromRSS() ([]string, error) {
	log.Printf("Fetching RSS feed: %s", RSSFeedURL)

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(RSSFeedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	urls := make([]string, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item.Link != "" {
			urls = append(urls, item.Link)
		}
	}

	log.Printf("Found %d releases in RSS feed", len(urls))
	return urls, nil
}

// FetchReleasesPage fetches release URLs from a specific pagination page
func (f *Fetcher) FetchReleasesPage(page int) ([]string, error) {
	url := fmt.Sprintf("%s?page=%d", ReleasesURL, page)
	log.Printf("Fetching releases list page %d: %s", page, url)

	html, err := f.FetchURL(url)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	urls := make([]string, 0)
	seen := make(map[string]bool)

	// Debug: count total links
	totalLinks := 0
	releaseLinks := 0

	// Find all links that point to /release/[slug]
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		totalLinks++
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		if strings.HasPrefix(href, "/release/") && href != "/release" && href != "/releases" {
			releaseLinks++
			fullURL := BaseURL + href
			if !seen[fullURL] {
				urls = append(urls, fullURL)
				seen[fullURL] = true
			}
		}
	})

	log.Printf("Found %d releases on page %d (total links: %d, release links: %d)", len(urls), page, totalLinks, releaseLinks)
	return urls, nil
}

// FetchReleases fetches release URLs from paginated listing
func (f *Fetcher) FetchReleases(maxPages int) ([]string, error) {
	log.Printf("Fetching releases (max pages: %d)", maxPages)

	allURLs := make(map[string]bool)
	consecutiveEmptyPages := 0
	maxEmptyPages := 3
	page := 0

	for consecutiveEmptyPages < maxEmptyPages {
		// Stop if we've reached the max pages limit
		if maxPages > 0 && page >= maxPages {
			log.Printf("Reached max pages limit of %d", maxPages)
			break
		}

		urls, err := f.FetchReleasesPage(page)
		if err != nil {
			log.Printf("Error fetching page %d: %v", page, err)
			consecutiveEmptyPages++
			page++
			continue
		}

		if len(urls) == 0 {
			consecutiveEmptyPages++
			page++
			continue
		}

		consecutiveEmptyPages = 0
		for _, url := range urls {
			allURLs[url] = true
		}

		page++

		// Safety limit
		if page > 100 {
			log.Printf("Reached safety limit of 100 pages")
			break
		}
	}

	// Convert map to slice
	result := make([]string, 0, len(allURLs))
	for url := range allURLs {
		result = append(result, url)
	}

	log.Printf("Found total of %d release URLs", len(result))
	return result, nil
}

// FetchRelease fetches a single release page
func (f *Fetcher) FetchRelease(url string) (string, error) {
	return f.FetchURL(url)
}
