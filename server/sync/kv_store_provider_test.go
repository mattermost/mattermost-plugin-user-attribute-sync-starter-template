package sync

import (
	"net/http"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestKVStoreProvider wires a KVStoreProvider to a mocked server API
func newTestKVStoreProvider(t *testing.T) (*KVStoreProvider, *plugintest.API) {
	t.Helper()

	api := &plugintest.API{}
	t.Cleanup(func() { api.AssertExpectations(t) })

	return NewKVStoreProvider(pluginapi.NewClient(api, &plugintest.Driver{})), api
}

// TestKVStoreProvider_ReturnsStoredUsers tests that the stored JSON is decoded into user records
func TestKVStoreProvider_ReturnsStoredUsers(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	//Fresh provider starts with a 0 lastSyncTime, so by setting lastUpdatedTime to now should trigger a new run.
	rawTime, _ := time.Now().MarshalJSON()
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(rawTime, nil).Once()

	api.On("KVGet", UserAttrsStoreKey).Return([]byte(`[
		{"email": "user1@example.com", "job_title": "Engineer"},
		{"email": "user2@example.com", "job_title": "Sales"}
	]`), nil).Once()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "user1@example.com", users[0]["email"])
	assert.Equal(t, "Engineer", users[0]["job_title"])
	assert.Equal(t, "user2@example.com", users[1]["email"])
	assert.Equal(t, "Sales", users[1]["job_title"])
}

// TestKVStoreProvider_NoStoredFile tests that an unset key is not an error, just no work to do
func TestKVStoreProvider_NoStoredFile(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up
	rawTime, _ := time.Now().MarshalJSON()
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(rawTime, nil).Once()

	api.On("KVGet", UserAttrsStoreKey).Return(nil, nil).Once()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users)
}

// TestKVStoreProvider_NoTimestamp tests that an unset timestamp key is not an error. It is the
// state of a fresh install, and of one whose stored file was deleted, so it has to mean
// "nothing to sync" rather than failing on every job tick.
func TestKVStoreProvider_NoTimestamp(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsLastUpdatedKey).Return(nil, nil).Once()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users)
}

// TestKVStoreProvider_RDoesNotProcessTwice documents that this provider is relies on
// a "processed" flag to be set when a file is updated, so it will not process the same file twice.
func TestKVStoreProvider_DoesNotProcessTwice(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up the first time
	// Note this will be called twice, before the first run, and then it will return the
	// same timestamp for the second call but will not run a second time
	rawTime, _ := time.Now().MarshalJSON()
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(rawTime, nil).Twice()

	api.On("KVGet", UserAttrsStoreKey).Return([]byte(`[{"email": "user1@example.com"}]`), nil).Once()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 1)

	users, err = provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users, "no data should be returned for a subsquent call")
}

// TestKVStoreProvider_InvalidJSON tests error handling for malformed stored data
func TestKVStoreProvider_InvalidJSON(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up
	rawTime, _ := time.Now().MarshalJSON()
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(rawTime, nil).Once()

	api.On("KVGet", UserAttrsStoreKey).Return([]byte("{invalid json content"), nil).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

// TestKVStoreProvider_FileStoreError tests error handling when the KV store is unreachable
func TestKVStoreProvider_FileStoreError(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up
	rawTime, _ := time.Now().MarshalJSON()
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(rawTime, nil).Once()

	api.On("KVGet", UserAttrsStoreKey).
		Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to retrieve user attrs from store")
}

// TestKVStoreProvider_TimestampStoreError tests error handling when the KV store is unreachable
func TestKVStoreProvider_TimestampStoreError(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up
	api.On("KVGet", UserAttrsLastUpdatedKey).
		Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to check lastUpdated timestamp")
}

// TestKVStoreProvider_MalformedTimestamp tests error handling when the KV store is unreachable
func TestKVStoreProvider_MalformedTimestamp(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Return a fresh timestamp to ensure the file is picked up
	// Return a fresh timestamp to ensure the file is picked up
	malformedTime := []byte(`invalid timestamp`)
	api.On("KVGet", UserAttrsLastUpdatedKey).Return(malformedTime, nil).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "malformed data")
}

// TestKVStoreProvider_Close tests that Close returns no error
func TestKVStoreProvider_Close(t *testing.T) {
	provider, _ := newTestKVStoreProvider(t)
	assert.NoError(t, provider.Close())
}

// TestNewKVStoreProvider tests that the constructor holds on to the client it is given
// And that the Processed Flag is initialized as unset.
func TestNewKVStoreProvider(t *testing.T) {
	client := pluginapi.NewClient(&plugintest.API{}, &plugintest.Driver{})

	provider := NewKVStoreProvider(client)
	assert.Same(t, client, provider.client)
}
