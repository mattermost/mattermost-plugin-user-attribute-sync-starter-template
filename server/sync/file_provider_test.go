package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileProvider_FirstSync tests that the first call to GetUserAttributes returns all users
func TestFileProvider_FirstSync(t *testing.T) {
	testData := []map[string]interface{}{
		{
			"email":      "user1@example.com",
			"department": "Engineering",
		},
		{
			"email":      "user2@example.com",
			"department": "Sales",
		},
	}

	tempFile, _ := writeJSONFile(t, "test_users.json", testData)

	// Create provider pointing to temp file
	provider := &FileProvider{
		filePath: tempFile,
	}

	// First sync should return all users
	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "user1@example.com", users[0]["email"])
	assert.Equal(t, "user2@example.com", users[1]["email"])

	// Verify state was updated
	assert.False(t, provider.lastReadTime.IsZero())
	assert.False(t, provider.lastModTime.IsZero())
}

// TestFileProvider_UnchangedFileReturnsEmpty tests that subsequent calls with no file changes return empty array
func TestFileProvider_UnchangedFileReturnsEmpty(t *testing.T) {
	testData := []map[string]interface{}{
		{"email": "user1@example.com"},
	}

	tempFile, _ := writeJSONFile(t, "test_users.json", testData)

	provider := &FileProvider{
		filePath: tempFile,
	}

	// First sync
	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 1)

	// Second sync without file modification should return empty
	users, err = provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users, "unchanged file should return empty array")
}

// TestFileProvider_ModifiedFileReturnsUsers tests that file modification triggers new data return
func TestFileProvider_ModifiedFileReturnsUsers(t *testing.T) {
	initialData := []map[string]interface{}{
		{"email": "user1@example.com"},
	}

	tempFile, _ := writeJSONFile(t, "test_users.json", initialData)

	provider := &FileProvider{
		filePath: tempFile,
	}

	// First sync
	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 1)

	// Sleep briefly to ensure modification time will be different
	time.Sleep(10 * time.Millisecond)

	// Modify the file
	updatedData := []map[string]interface{}{
		{"email": "user1@example.com"},
		{"email": "user2@example.com"},
	}
	jsonData, err := json.Marshal(updatedData)
	require.NoError(t, err)
	err = os.WriteFile(tempFile, jsonData, 0600)
	require.NoError(t, err)

	// Second sync should return updated data
	users, err = provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 2, "modified file should return all users")
	assert.Equal(t, "user1@example.com", users[0]["email"])
	assert.Equal(t, "user2@example.com", users[1]["email"])
}

// TestFileProvider_FileNotFound tests error handling when file doesn't exist
func TestFileProvider_FileNotFound(t *testing.T) {
	provider := &FileProvider{
		filePath: "/nonexistent/path/users.json",
	}

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to stat file")
}

// TestFileProvider_InvalidJSON tests error handling for malformed JSON
func TestFileProvider_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "invalid.json")

	// Write invalid JSON
	err := os.WriteFile(tempFile, []byte("{invalid json content"), 0600)
	require.NoError(t, err)

	provider := &FileProvider{
		filePath: tempFile,
	}

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

// TestFileProvider_EmptyFile tests handling of empty but valid JSON array
func TestFileProvider_EmptyFile(t *testing.T) {
	tempFile, _ := writeJSONFile(t, "empty.json", []map[string]interface{}{})

	provider := &FileProvider{
		filePath: tempFile,
	}

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users)
}

// TestFileProvider_Close tests that Close returns no error
func TestFileProvider_Close(t *testing.T) {
	provider := NewFileProvider()
	err := provider.Close()
	assert.NoError(t, err)
}

// TestNewFileProvider tests that the constructor sets the correct default path
func TestNewFileProvider(t *testing.T) {
	provider := NewFileProvider()
	assert.Equal(t, defaultDataFilePath, provider.filePath)
	assert.True(t, provider.lastReadTime.IsZero())
	assert.True(t, provider.lastModTime.IsZero())
}

// Helper function to write JSON test data to a temp file.
// Creates a temp directory, writes the file, and returns both the full file path
// and the directory path for convenience in tests that need to modify the file.
func writeJSONFile(t *testing.T, filename string, data interface{}) (filePath, dirPath string) {
	t.Helper()
	tempDir := t.TempDir()
	fullPath := filepath.Join(tempDir, filename)

	jsonData, err := json.Marshal(data)
	require.NoError(t, err)

	err = os.WriteFile(fullPath, jsonData, 0600)
	require.NoError(t, err)

	return fullPath, tempDir
}
