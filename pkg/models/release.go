package models

import "time"

// Minister represents a government minister associated with a release
type Minister struct {
	Name       string `json:"name"`
	ProfileURL string `json:"profile_url"`
}

// Attachment represents a downloadable document attached to a release
type Attachment struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// Release represents a press release from Beehive.govt.nz
type Release struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	URL         string       `json:"url"`
	Date        string       `json:"date"`          // ISO date in NZ timezone: YYYY-MM-DD
	DateTime    string       `json:"date_time"`     // Full UTC timestamp (ISO 8601)
	Ministers   []Minister   `json:"ministers"`
	Portfolios  []string     `json:"portfolios"`
	Content     string       `json:"content"`       // HTML content
	ContentText string       `json:"content_text"`  // Plain text content
	Attachments []Attachment `json:"attachments"`
	ScrapedAt   time.Time    `json:"scraped_at"`
	Metadata    Metadata     `json:"metadata"`
}

// Metadata contains additional information about the release
type Metadata struct {
	Government string `json:"government"` // e.g., "National/ACT/NZ First Coalition - 2023-2026"
}
