package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/marcus-crane/beehive-exporter/pkg/models"
)

const (
	DataDir      = "json"
	RawDir       = "raw"
	MarkdownDir  = "markdown"
	IndexFile    = "index.json"
	URLIndexFile = "url-index.json"
)

// Index represents the master index of all releases
type Index struct {
	TotalCount   int       `json:"total_count"`
	LastSyncAt   time.Time `json:"last_sync_at"`
	ReleaseCount map[string]int `json:"release_count_by_year"`
}

// Storage handles saving and loading releases
type Storage struct {
	baseDir string
}

// New creates a new Storage instance
func New(baseDir string) *Storage {
	if baseDir == "" {
		baseDir = "."
	}
	return &Storage{baseDir: baseDir}
}

// SaveRelease saves a release to a JSON file organized by content type/year/month
func (s *Storage) SaveRelease(release *models.Release, contentType string) error {
	year := release.Time.Format("2006")
	month := release.Time.Format("01")
	day := release.Time.Format("2006-01-02")

	// Create directory structure: json/{contentType}/YYYY/MM/
	dir := filepath.Join(s.baseDir, DataDir, contentType, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create filename with date prefix: YYYY-MM-DD-id.json
	filename := filepath.Join(dir, day+"-"+release.ID+".json")

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temp file first, then rename (atomic write)
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, filename); err != nil {
		os.Remove(tempFile) // Clean up temp file on error
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// SaveRawHTML saves the raw HTML to a file for potential future reprocessing
// It trims the HTML to just the main container to reduce file size
func (s *Storage) SaveRawHTML(id string, releaseTime time.Time, html string, contentType string) error {
	year := releaseTime.Format("2006")
	month := releaseTime.Format("01")
	day := releaseTime.Format("2006-01-02")

	dir := filepath.Join(s.baseDir, RawDir, contentType, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create raw directory: %w", err)
	}

	// Trim HTML to just the main container
	trimmedHTML := trimToMainContainer(html)

	// Filename with date prefix: YYYY-MM-DD-id.html
	filename := filepath.Join(dir, day+"-"+id+".html")
	if err := os.WriteFile(filename, []byte(trimmedHTML), 0644); err != nil {
		return fmt.Errorf("failed to write raw HTML: %w", err)
	}

	return nil
}

// trimToMainContainer extracts just the main content element from the HTML
// This removes navigation, scripts, and other boilerplate, reducing file size significantly
// Falls back to original HTML if no known container is found
func trimToMainContainer(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	// Try various selectors in order of preference
	selectors := []string{
		"main.main-container",
		"div.ds-three-col__main",
		"article.release",
		"article",
		"div.content",
		"#content",
	}

	for _, selector := range selectors {
		container := doc.Find(selector).First()
		if container.Length() > 0 {
			trimmed, err := container.Html()
			if err == nil && len(trimmed) > 100 { // Sanity check: must have some content
				return trimmed
			}
		}
	}

	// No container found - return original
	return html
}

// LoadRelease loads a release from a JSON file
func (s *Storage) LoadRelease(id string, contentType string) (*models.Release, error) {
	// Try to find the file (we need to search through year/month directories)
	var foundPath string

	searchPath := filepath.Join(s.baseDir, DataDir, contentType)
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, id+".json") {
			foundPath = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if foundPath == "" {
		return nil, fmt.Errorf("release not found: %s", id)
	}

	data, err := os.ReadFile(foundPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var release models.Release
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return &release, nil
}

// ReleaseExists checks if a release already exists (without loading the full file)
func (s *Storage) ReleaseExists(id string, contentType string) bool {
	found := false
	searchPath := filepath.Join(s.baseDir, DataDir, contentType)
	filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, id+".json") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// LoadAllReleases loads all releases from the data directory
func (s *Storage) LoadAllReleases() ([]*models.Release, error) {
	releases := make([]*models.Release, 0)

	err := filepath.WalkDir(filepath.Join(s.baseDir, DataDir), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		var release models.Release
		if err := json.Unmarshal(data, &release); err != nil {
			return fmt.Errorf("failed to unmarshal %s: %w", path, err)
		}

		releases = append(releases, &release)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return releases, nil
}

// UpdateIndex updates the master index file
func (s *Storage) UpdateIndex() error {
	releases, err := s.LoadAllReleases()
	if err != nil {
		return err
	}

	index := Index{
		TotalCount:   len(releases),
		LastSyncAt:   time.Now(),
		ReleaseCount: make(map[string]int),
	}

	// Count releases by year
	for _, release := range releases {
		year := release.Time.Format("2006")
		index.ReleaseCount[year]++
	}

	// Save index
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	indexPath := filepath.Join(s.baseDir, IndexFile)
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// URLIndex represents URLs grouped by government and year
type URLIndex map[string]map[string][]string // government -> year -> urls

// RawHTMLFile represents a raw HTML file with its path and metadata
type RawHTMLFile struct {
	Path        string
	ID          string
	ContentType string
	URL         string // Will be empty, needs to be provided separately
}

// LoadAllRawHTML returns paths to all raw HTML files
func (s *Storage) LoadAllRawHTML() ([]RawHTMLFile, error) {
	var files []RawHTMLFile

	rawBasePath := filepath.Join(s.baseDir, RawDir)

	// Use WalkDir instead of Walk - it's much faster because it doesn't call Stat on every file
	err := filepath.WalkDir(rawBasePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}

		// Extract ID from filename: YYYY-MM-DD-id.html -> id
		base := filepath.Base(path)
		base = strings.TrimSuffix(base, ".html")
		// Remove date prefix (YYYY-MM-DD-)
		if len(base) > 11 && base[4] == '-' && base[7] == '-' && base[10] == '-' {
			base = base[11:]
		}

		// Extract content type from path: raw/{contentType}/YYYY/MM/file.html
		relPath, _ := filepath.Rel(rawBasePath, path)
		parts := strings.Split(relPath, string(filepath.Separator))
		contentType := "releases" // default
		if len(parts) >= 1 {
			contentType = parts[0]
		}

		files = append(files, RawHTMLFile{
			Path:        path,
			ID:          base,
			ContentType: contentType,
		})
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// ReadRawHTML reads the content of a raw HTML file
func (s *Storage) ReadRawHTML(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RawHTMLExistsWithDate checks if raw HTML exists for a given ID and date using direct path lookup
func (s *Storage) RawHTMLExistsWithDate(id string, releaseDate time.Time, contentType string) bool {
	year := releaseDate.Format("2006")
	month := releaseDate.Format("01")
	day := releaseDate.Format("2006-01-02")

	path := filepath.Join(s.baseDir, RawDir, contentType, year, month, day+"-"+id+".html")
	_, err := os.Stat(path)
	return err == nil
}

// GenerateURLIndex builds an index of all release URLs grouped by government and year
func (s *Storage) GenerateURLIndex() error {
	releases, err := s.LoadAllReleases()
	if err != nil {
		return err
	}

	index := make(URLIndex)

	for _, release := range releases {
		gov := release.Government
		year := release.Time.Format("2006")

		if index[gov] == nil {
			index[gov] = make(map[string][]string)
		}
		index[gov][year] = append(index[gov][year], release.URL)
	}

	// Sort URLs within each year for consistent output
	for gov := range index {
		for year := range index[gov] {
			urls := index[gov][year]
			sort.Strings(urls)
			index[gov][year] = urls
		}
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal URL index: %w", err)
	}

	indexPath := filepath.Join(s.baseDir, URLIndexFile)
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write URL index: %w", err)
	}

	return nil
}
