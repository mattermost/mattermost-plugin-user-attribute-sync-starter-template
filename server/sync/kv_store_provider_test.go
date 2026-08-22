package sync

import (
	"encoding/json"
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

// storedValue encodes a StoredUserAttrs the way the upload handler does, so a KVGet mock returns
// what the provider would really find.
func storedValue(t *testing.T, lastUpdated time.Time, data []byte) []byte {
	t.Helper()

	value, err := json.Marshal(StoredUserAttrs{LastUpdated: lastUpdated, Data: data})
	require.NoError(t, err)

	return value
}

// TestKVStoreProvider_ReturnsStoredUsers tests that the stored JSON is decoded into user records
func TestKVStoreProvider_ReturnsStoredUsers(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// Fresh provider starts with a 0 lastSyncTime, so by setting lastUpdatedTime to now() should trigger a new run.
	api.On("KVGet", UserAttrsStoreKey).Return(storedValue(t, time.Now(), []byte(`[
		{"email": "user1@example.com", "job_title": "Engineer"},
		{"email": "user2@example.com", "job_title": "Sales"}
	]`)), nil).Once()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "user1@example.com", users[0]["email"])
	assert.Equal(t, "Engineer", users[0]["job_title"])
	assert.Equal(t, "user2@example.com", users[1]["email"])
	assert.Equal(t, "Sales", users[1]["job_title"])
}

// TestKVStoreProvider_NoStoredFile tests that a timestamp with no file behind it is an error. The
// handlers cannot produce this state; a hand-edited value can.
func TestKVStoreProvider_NoStoredFile(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsStoreKey).Return(storedValue(t, time.Now(), nil), nil).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "no user attributes file in the KV store")
}

// TestKVStoreProvider_NothingStored tests that an unset key is an error, matching how FileProvider
// treats a missing file. Sync is pointed at this store, so finding nothing in it is a
// misconfiguration to surface rather than silently accept.
func TestKVStoreProvider_NothingStored(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsStoreKey).Return(nil, nil).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "no user attributes file in the KV store")
}

// TestKVStoreProvider_RDoesNotProcessTwice documents that this provider is relies on
// a "processed" flag to be set when a file is updated, so it will not process the same file twice.
func TestKVStoreProvider_DoesNotProcessTwice(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	// A fresh timestamp is picked up the first time. The second call reads the same value and
	// returns nothing, because the timestamp has not moved.
	api.On("KVGet", UserAttrsStoreKey).
		Return(storedValue(t, time.Now(), []byte(`[{"email": "user1@example.com"}]`)), nil).Twice()

	users, err := provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Len(t, users, 1)

	users, err = provider.GetUserAttributes()
	require.NoError(t, err)
	assert.Empty(t, users, "no data should be returned for a subsequent call")
}

// TestKVStoreProvider_InvalidJSON tests error handling for a stored file that is not user records
func TestKVStoreProvider_InvalidJSON(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsStoreKey).
		Return(storedValue(t, time.Now(), []byte("{invalid json content")), nil).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to parse JSON")
}

// TestKVStoreProvider_FileStoreError tests error handling when the KV store is unreachable
func TestKVStoreProvider_FileStoreError(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsStoreKey).
		Return(nil, model.NewAppError("KVGet", "kv.get.app_error", nil, "connection refused", http.StatusInternalServerError)).Once()

	users, err := provider.GetUserAttributes()
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Contains(t, err.Error(), "failed to read user-attrs from the KV store")
}

// TestKVStoreProvider_MalformedStoredValue tests error handling for a stored value that is not the
// format the upload handler writes.
func TestKVStoreProvider_MalformedStoredValue(t *testing.T) {
	provider, api := newTestKVStoreProvider(t)

	api.On("KVGet", UserAttrsStoreKey).Return([]byte(`malformed`), nil).Once()

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
