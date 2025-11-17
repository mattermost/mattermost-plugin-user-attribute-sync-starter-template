package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const defaultDataFilePath = "data/user_attributes.json"

// FileProvider implements AttributeProvider by reading user attribute data from a JSON file.
// It supports incremental synchronization by tracking the file's modification time and only
// returning data when the file has been modified since the last read.
type FileProvider struct {
	filePath     string
	lastReadTime time.Time
	lastModTime  time.Time
}

// NewFileProvider creates a new FileProvider that reads from the default data file path.
func NewFileProvider() *FileProvider {
	return &FileProvider{
		filePath: defaultDataFilePath,
	}
}

// GetUserAttributes reads user attribute data from the JSON file.
// On the first call, it returns all users from the file.
// On subsequent calls, it checks if the file has been modified since the last read:
//   - If modified: reads and returns the updated user data
//   - If unchanged: returns an empty array to signal no new data
func (f *FileProvider) GetUserAttributes() ([]map[string]interface{}, error) {
	// Get file modification time
	fileInfo, err := os.Stat(f.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", f.filePath, err)
	}

	modTime := fileInfo.ModTime()

	// If file hasn't been modified since last read, return empty array
	if !f.lastModTime.IsZero() && !modTime.After(f.lastModTime) {
		return []map[string]interface{}{}, nil
	}

	// Read the file
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", f.filePath, err)
	}

	// Parse JSON
	var users []map[string]interface{}
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", f.filePath, err)
	}

	// Update tracking state
	f.lastReadTime = time.Now()
	f.lastModTime = modTime

	return users, nil
}

// Close releases any resources held by the provider.
// For FileProvider, this is a no-op as no persistent resources are held.
func (f *FileProvider) Close() error {
	return nil
}
