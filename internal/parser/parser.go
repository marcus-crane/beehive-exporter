package parser

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcuswhybrow/beehive-exports/pkg/models"
)

// Parse parses HTML content and extracts release data
func Parse(html, releaseURL string) (*models.Release, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	release := &models.Release{
		URL:       releaseURL,
		ScrapedAt: time.Now(),
	}

	// Extract ID from URL
	release.ID = extractIDFromURL(releaseURL)

	// Extract title
	release.Title = strings.TrimSpace(doc.Find("h1").First().Text())

	// Extract date and datetime
	release.Date, release.DateTime = extractDate(doc)

	// Extract ministers
	release.Ministers = extractMinisters(doc)

	// Extract portfolios
	release.Portfolios = extractPortfolios(doc)

	// Extract content
	release.Content, release.ContentText = extractContent(doc)

	// Extract attachments
	release.Attachments = extractAttachments(doc)

	// Determine government term
	release.Metadata.Government = determineGovernment(release.Date)

	return release, nil
}

// extractIDFromURL extracts the slug/ID from a release URL
func extractIDFromURL(releaseURL string) string {
	u, err := url.Parse(releaseURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(path.Base(u.Path), "/")
}

// extractDate extracts and formats the publication date
// Returns both date (YYYY-MM-DD) and full datetime (ISO 8601)
func extractDate(doc *goquery.Document) (string, string) {
	dateText := ""
	fullDateTime := ""

	// Try to find datetime in time elements with datetime attribute
	doc.Find("time[datetime]").Each(func(i int, s *goquery.Selection) {
		if dt, exists := s.Attr("datetime"); exists && dt != "" {
			fullDateTime = dt
			return
		}
	})

	// Also try to find readable date text
	doc.Find("time, .date, .published").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" && dateText == "" {
			dateText = text
		}
	})

	// If not found, look for date patterns in text
	if dateText == "" {
		doc.Find("*").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if matched, _ := regexp.MatchString(`\d{1,2}\s+\w+\s+\d{4}`, text); matched {
				dateText = text
				return
			}
		})
	}

	// Parse and extract the date portion
	var parsedDate time.Time
	var err error

	// Load NZ timezone
	nzLocation, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		// Fallback to UTC+12 if timezone loading fails
		nzLocation = time.FixedZone("NZST", 12*60*60)
	}

	// First try to parse the full datetime if we have it
	if fullDateTime != "" {
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
		}
		for _, format := range formats {
			if parsedDate, err = time.Parse(format, fullDateTime); err == nil {
				// Convert to NZ time for the date field
				nzTime := parsedDate.In(nzLocation)
				return nzTime.Format("2006-01-02"), fullDateTime
			}
		}
	}

	// Fall back to parsing readable date text
	if dateText != "" {
		formats := []string{
			"2 January 2006",
			"02 January 2006",
			"2006-01-02",
		}

		for _, format := range formats {
			if parsedDate, err = time.Parse(format, dateText); err == nil {
				date := parsedDate.Format("2006-01-02")
				if fullDateTime == "" {
					fullDateTime = parsedDate.Format(time.RFC3339)
				}
				return date, fullDateTime
			}
		}
	}

	// Fallback - use current NZ time
	now := time.Now().In(nzLocation)
	return now.Format("2006-01-02"), now.UTC().Format(time.RFC3339)
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

// extractContent extracts the main content body
func extractContent(doc *goquery.Document) (html string, text string) {
	// Find the main body content by looking for specific Drupal field classes
	// There may be multiple matches, so find the one with the most content
	bodyFields := doc.Find("div.field.field--name-body, div.prose")

	var largestMatch *goquery.Selection
	maxLength := 0

	bodyFields.Each(func(i int, s *goquery.Selection) {
		content := strings.TrimSpace(s.Text())
		if len(content) > maxLength {
			maxLength = len(content)
			largestMatch = s
		}
	})

	if largestMatch != nil && maxLength > 0 {
		// Get HTML from the largest match
		html, _ = largestMatch.Html()

		// Get clean text
		text = strings.TrimSpace(largestMatch.Text())

		// Clean up excessive whitespace in text
		text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

		return html, text
	}

	// Fallback: try to find main content area
	content := doc.Find(".content, .release-content, main article, article").First()

	if content.Length() == 0 {
		// Last resort: get body paragraphs
		var textParts []string
		doc.Find("p").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if len(text) > 20 { // Skip very short paragraphs (likely navigation)
				textParts = append(textParts, text)
			}
		})
		text = strings.Join(textParts, "\n\n")
		return "", text
	}

	// Remove navigation, headers, footers, social buttons
	content.Find("nav, header, footer, .menu, .navigation, .social, aside").Remove()

	html, _ = content.Html()
	text = strings.TrimSpace(content.Text())

	// Clean up excessive whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return html, text
}

// extractAttachments extracts downloadable attachments
func extractAttachments(doc *goquery.Document) []models.Attachment {
	attachments := make([]models.Attachment, 0)

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Look for PDF, DOC, DOCX, etc.
		lower := strings.ToLower(href)
		if !strings.Contains(lower, ".pdf") &&
			!strings.Contains(lower, ".doc") &&
			!strings.Contains(lower, ".docx") &&
			!strings.Contains(lower, ".xls") &&
			!strings.Contains(lower, ".xlsx") {
			return
		}

		title := strings.TrimSpace(s.Text())
		if title == "" {
			title = path.Base(href)
		}

		fileType := ""
		if strings.Contains(lower, ".pdf") {
			fileType = "pdf"
		} else if strings.Contains(lower, ".doc") {
			fileType = "doc"
		} else if strings.Contains(lower, ".xls") {
			fileType = "xls"
		}

		fullURL := href
		if !strings.HasPrefix(fullURL, "http") {
			fullURL = "https://www.beehive.govt.nz" + fullURL
		}

		attachments = append(attachments, models.Attachment{
			Title: title,
			URL:   fullURL,
			Type:  fileType,
		})
	})

	return attachments
}

// determineGovernment determines the government term based on the date
func determineGovernment(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "Unknown"
	}

	year := t.Year()

	switch {
	case year >= 2023:
		return "National/ACT/New Zealand First Coalition Government - 2023-2026"
	case year >= 2020:
		return "Labour Government - 2020-2023"
	case year >= 2017:
		return "Labour/New Zealand First Coalition Government - 2017-2020"
	case year >= 2014:
		return "National Government - 2014-2017"
	case year >= 2011:
		return "National/ACT/United Future Government - 2011-2014"
	case year >= 2008:
		return "National/ACT/Māori Party/United Future Government - 2008-2011"
	default:
		return fmt.Sprintf("Government - %d", year)
	}
}
