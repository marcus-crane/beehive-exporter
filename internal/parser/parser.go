package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcus-crane/beehive-exports/pkg/models"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Parse parses HTML content and extracts release data
func Parse(rawHTML, releaseURL string) (*models.Release, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract main content div to avoid storing JS bundles and redundant data
	mainContent, err := doc.Find("div.ds-three-col__main").First().Html()
	if err != nil {
		mainContent = ""
	}

	release := &models.Release{
		URL:       releaseURL,
		ScrapedAt: time.Now(),
		Content:   mainContent,
	}

	// Extract ID from URL
	release.ID = ExtractIDFromURL(releaseURL)

	// Extract title
	release.Title = strings.TrimSpace(doc.Find("h1").First().Text())

	// Extract time
	release.Time = extractTime(doc)

	// Extract ministers
	release.Ministers = extractMinisters(doc)

	// Extract portfolios
	release.Portfolios = extractPortfolios(doc)

	// Extract plain text content
	release.ContentText = extractContentText(doc)

	// Extract attachments
	release.Attachments = extractAttachments(doc)

	// Determine government term
	release.Government = determineGovernment(release.Time)

	return release, nil
}

// ExtractIDFromURL extracts and normalizes the slug/ID from a release URL
func ExtractIDFromURL(releaseURL string) string {
	// URL format: https://www.beehive.govt.nz/release/some-slug
	parts := strings.Split(releaseURL, "/")
	if len(parts) == 0 {
		return ""
	}
	id := parts[len(parts)-1]

	// Decode URL-encoded characters
	if decoded, err := url.QueryUnescape(id); err == nil {
		id = decoded
	}

	// Normalize Unicode: NFD decomposition strips diacritics (ā → a)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)))
	id, _, _ = transform.String(t, id)

	// Remove any remaining non-ASCII punctuation like curly quotes
	id = strings.Map(func(r rune) rune {
		if r > 127 {
			return -1
		}
		return r
	}, id)

	return id
}

// extractTime extracts the publication time
func extractTime(doc *goquery.Document) time.Time {
	// Try to find datetime in time elements with datetime attribute
	var fullDateTime string
	doc.Find("time[datetime]").Each(func(i int, s *goquery.Selection) {
		if dt, exists := s.Attr("datetime"); exists && dt != "" {
			fullDateTime = dt
		}
	})

	if fullDateTime != "" {
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, fullDateTime); err == nil {
				return t
			}
		}
	}

	// Fall back to parsing readable date text
	var dateText string
	doc.Find("time, .date, .published").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" && dateText == "" {
			dateText = text
		}
	})

	if dateText != "" {
		formats := []string{
			"2 January 2006",
			"02 January 2006",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, dateText); err == nil {
				return t
			}
		}
	}

	return time.Now()
}

// extractMinisters extracts minister information
func extractMinisters(doc *goquery.Document) []models.Minister {
	ministers := make([]models.Minister, 0)
	seen := make(map[string]bool)

	// First, look for modern minister links with profile URLs
	doc.Find("a[href*='/minister/']").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		name := strings.TrimSpace(s.Text())
		if name == "" {
			// Try to find name in nearby text or img alt
			name = strings.TrimSpace(s.Find("img").AttrOr("alt", ""))
		}

		if name != "" && !seen[name] {
			profileURL := href
			if !strings.HasPrefix(profileURL, "http") {
				profileURL = "https://www.beehive.govt.nz" + profileURL
			}

			ministers = append(ministers, models.Minister{
				Name:       name,
				ProfileURL: profileURL,
			})
			seen[name] = true
		}
	})

	// If no modern minister links found, look for archived minister patterns
	if len(ministers) == 0 {
		ministers = extractArchivedMinisters(doc, seen)
	}

	return ministers
}

// extractArchivedMinisters handles older releases where ministers are plain text
func extractArchivedMinisters(doc *goquery.Document, seen map[string]bool) []models.Minister {
	ministers := make([]models.Minister, 0)

	// Pattern 1: Archived ministers inside the ministers list
	// Structure: <ul class="meta--ministers"><li><span class="is-archived">Denis Marshall</span></li></ul>
	doc.Find("ul.meta--ministers li span.is-archived").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Text())
		if name != "" && !seen[name] {
			ministers = append(ministers, models.Minister{
				Name:       name,
				ProfileURL: "", // No profile URL for archived ministers
			})
			seen[name] = true
		}
	})

	// Pattern 2: Ministers in list items before the main content (fallback)
	if len(ministers) == 0 {
		mainContent := doc.Find("div.ds-three-col__main, div.field--name-body, article").First()
		if mainContent.Length() > 0 {
			// Look at the first few list items - they often contain minister names
			mainContent.Find("ul").First().Find("li").Each(func(i int, s *goquery.Selection) {
				// Only check the first few items
				if i > 2 {
					return
				}

				text := strings.TrimSpace(s.Text())
				if isLikelyMinisterName(text) && !seen[text] {
					ministers = append(ministers, models.Minister{
						Name:       text,
						ProfileURL: "",
					})
					seen[text] = true
				}
			})
		}
	}

	// Pattern 3: Look for field labels like "Minister:" followed by text
	doc.Find(".field__label").Each(func(i int, s *goquery.Selection) {
		label := strings.TrimSpace(s.Text())
		if strings.Contains(strings.ToLower(label), "minister") {
			// Look for the value in a sibling element
			value := s.Parent().Find(".field__item").Text()
			if value == "" {
				value = s.Next().Text()
			}
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				ministers = append(ministers, models.Minister{
					Name:       value,
					ProfileURL: "",
				})
				seen[value] = true
			}
		}
	})

	return ministers
}

// isLikelyMinisterName checks if text looks like a minister name
func isLikelyMinisterName(text string) bool {
	if text == "" || len(text) > 100 {
		return false
	}

	// Common minister name prefixes
	prefixes := []string{"Hon", "Rt Hon", "Hon.", "Rt Hon.", "Dr", "Sir", "Dame"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix+" ") {
			return true
		}
	}

	// Check if it looks like a name (2-4 words, capitalized)
	words := strings.Fields(text)
	if len(words) < 2 || len(words) > 5 {
		return false
	}

	// All words should start with uppercase (names)
	for _, word := range words {
		if len(word) == 0 {
			continue
		}
		// Skip common titles/suffixes
		if word == "MP" || word == "KC" || word == "QC" || word == "CNZM" || word == "GNZM" {
			continue
		}
		if !unicode.IsUpper(rune(word[0])) {
			return false
		}
	}

	// Exclude obvious non-names
	lowerText := strings.ToLower(text)
	excludePatterns := []string{"portfolio", "minister for", "minister of", "office", "department", "conservation", "health", "education", "finance"}
	for _, pattern := range excludePatterns {
		if strings.Contains(lowerText, pattern) {
			return false
		}
	}

	return true
}

// extractPortfolios extracts portfolio tags
func extractPortfolios(doc *goquery.Document) []string {
	portfolios := make([]string, 0)
	seen := make(map[string]bool)

	// Look for modern portfolio links
	doc.Find("a[href*='/portfolio/'], .portfolio, .portfolio-tag").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Text())
		if name != "" && !seen[name] {
			portfolios = append(portfolios, name)
			seen[name] = true
		}
	})

	// Archived portfolios inside em.tag--portfolio
	doc.Find("em.tag--portfolio span.is-archived, .tag--portfolio span.is-archived").Each(func(i int, s *goquery.Selection) {
		name := strings.TrimSpace(s.Text())
		if name != "" && !seen[name] {
			portfolios = append(portfolios, name)
			seen[name] = true
		}
	})

	// Archived portfolios as standalone span.is-archived outside minister list
	// These appear after the ministers list but before the content body
	doc.Find("div.ds-three-col__main > span.is-archived").Each(func(i int, s *goquery.Selection) {
		// Skip if inside ministers list
		if s.ParentsFiltered("ul.meta--ministers").Length() > 0 {
			return
		}
		name := strings.TrimSpace(s.Text())
		if name != "" && !seen[name] {
			portfolios = append(portfolios, name)
			seen[name] = true
		}
	})

	return portfolios
}

// extractContentText extracts the plain text content
func extractContentText(doc *goquery.Document) string {
	content := doc.Find("div.prose.field--name-body").First()

	if content.Length() == 0 {
		content = doc.Find("div.field--name-body").First()
	}

	if content.Length() == 0 {
		content = doc.Find("article .content, .release-content").First()
	}

	if content.Length() == 0 {
		return ""
	}

	text := strings.TrimSpace(content.Text())
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return text
}

// extractAttachments extracts downloadable attachment URLs from the sidebar
func extractAttachments(doc *goquery.Document) []string {
	var attachments []string
	seen := make(map[string]bool)

	// Attachments live in the "Related Documents" sidebar section
	doc.Find("aside.article__related a[href], aside.ds-three-col__right .file a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		fullURL := href
		if !strings.HasPrefix(fullURL, "http") {
			fullURL = "https://www.beehive.govt.nz" + fullURL
		}

		if !seen[fullURL] {
			attachments = append(attachments, fullURL)
			seen[fullURL] = true
		}
	})

	return attachments
}

// determineGovernment determines the government term based on the time
func determineGovernment(t time.Time) string {
	year := t.Year()

	switch {
	case year >= 2023:
		return "National/ACT/New Zealand First Coalition Government"
	case year >= 2020:
		return "Labour Government"
	case year >= 2017:
		return "Labour/New Zealand First Coalition Government"
	case year >= 2014:
		return "National Government"
	case year >= 2011:
		return "National/ACT/United Future Government"
	case year >= 2008:
		return "National/ACT/Māori Party/United Future Government"
	default:
		return fmt.Sprintf("Government - %d", year)
	}
}
