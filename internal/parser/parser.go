package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcuswhybrow/beehive-exports/pkg/models"
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

	release := &models.Release{
		URL:       releaseURL,
		ScrapedAt: time.Now(),
		Content:   rawHTML, // Store full page HTML
	}

	// Extract ID from URL
	release.ID = extractIDFromURL(releaseURL)

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

// extractIDFromURL extracts the slug/ID from a release URL
func extractIDFromURL(releaseURL string) string {
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

	// Look for minister cards or links
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

	return ministers
}

// extractPortfolios extracts portfolio tags
func extractPortfolios(doc *goquery.Document) []string {
	portfolios := make([]string, 0)
	seen := make(map[string]bool)

	// Look for portfolio links or tags
	doc.Find("a[href*='/portfolio/'], .portfolio, .portfolio-tag").Each(func(i int, s *goquery.Selection) {
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

// extractAttachments extracts downloadable attachment URLs
func extractAttachments(doc *goquery.Document) []string {
	var attachments []string
	extensions := []string{".pdf", ".doc", ".docx", ".xls", ".xlsx"}

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		lowerHref := strings.ToLower(href)
		isAttachment := false
		for _, ext := range extensions {
			if strings.HasSuffix(lowerHref, ext) {
				isAttachment = true
				break
			}
		}
		if !isAttachment {
			return
		}

		fullURL := href
		if !strings.HasPrefix(fullURL, "http") {
			fullURL = "https://www.beehive.govt.nz" + fullURL
		}

		attachments = append(attachments, fullURL)
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
