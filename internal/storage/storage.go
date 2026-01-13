package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marcus-crane/beehive-exports/pkg/models"
)

const (
	DataDir   = "content/json"
	RawDir    = "content/raw"
	IndexFile = "content/index.json"
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

// SaveRelease saves a release to a JSON file organized by year/month
func (s *Storage) SaveRelease(release *models.Release) error {
	year := release.Time.Format("2006")
	month := release.Time.Format("01")

	// Create directory structure: data/releases/YYYY/MM/
	dir := filepath.Join(s.baseDir, DataDir, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create filename from ID
	filename := filepath.Join(dir, release.ID+".json")

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
func (s *Storage) SaveRawHTML(id string, releaseTime time.Time, html string) error {
	year := releaseTime.Format("2006")
	month := releaseTime.Format("01")

	dir := filepath.Join(s.baseDir, RawDir, year, month)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create raw directory: %w", err)
	}

	filename := filepath.Join(dir, id+".html")
	if err := os.WriteFile(filename, []byte(html), 0644); err != nil {
		return fmt.Errorf("failed to write raw HTML: %w", err)
	}

	return nil
}

// LoadRelease loads a release from a JSON file
func (s *Storage) LoadRelease(id string) (*models.Release, error) {
	// Try to find the file (we need to search through year/month directories)
	var foundPath string

	err := filepath.Walk(filepath.Join(s.baseDir, DataDir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, id+".json") {
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
func (s *Storage) ReleaseExists(id string) bool {
	found := false
	filepath.Walk(filepath.Join(s.baseDir, DataDir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, id+".json") {
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

	err := filepath.Walk(filepath.Join(s.baseDir, DataDir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".json") {
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
