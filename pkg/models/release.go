package models

import "time"

// Minister represents a government minister associated with a release
type Minister struct {
	Name       string `json:"name"`
	ProfileURL string `json:"profile_url"`
}

// Release represents a press release from Beehive.govt.nz
type Release struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	Time        time.Time  `json:"time"`
	ContentType string     `json:"content_type"` // releases, speeches, features, diaries
	Ministers   []Minister `json:"ministers"`
	Portfolios  []string   `json:"portfolios"`
	Government  string     `json:"government"`
	Content     string     `json:"content"`      // Full page HTML
	ContentText string     `json:"content_text"` // Plain text content
	Attachments []string   `json:"attachments"`
	ScrapedAt   time.Time  `json:"scraped_at"`
}
