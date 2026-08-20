package sync

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// The two KV keys this provider reads. They are written by the HTTP handlers in
// server/http_hooks.go and are a matched pair: the file is the data, and the timestamp is what
// tells this provider the data is new. Writing or removing one without the other leaves a file
// that is never synced, or a timestamp pointing at nothing — GetUserAttributes reports the latter
// as an error rather than papering over it.
const UserAttrsStoreKey = "user-attrs-file"
const UserAttrsLastUpdatedKey = "user-attrs-last-updated"

// KVStoreProvider implements AttributeProvider by reading user attribute data an admin uploaded
// through the System Console, which the plugin stores in Mattermost's KV store.  THe KV Store lives in
// Mattermost's Postgres database, which is a more stable storage location than file systems in a
// cloud environment.
//
// Incremental synchronization works the similar to FileProvider's, with a stored timestamp
// standing in for the file's modification time.
type KVStoreProvider struct {
	client *pluginapi.Client

	// lastSynced is when this provider last read the stored file. It is in-memory only, so a
	// plugin restart re-syncs the stored file once.
	lastSynced time.Time
}

func NewKVStoreProvider(client *pluginapi.Client) *KVStoreProvider {
	return &KVStoreProvider{
		client: client,
	}
}

// GetUserAttributes reads the user attribute data an admin uploaded, from the KV store.
// On the first call, it returns everything stored.
// On subsequent calls, it checks whether a newer file has been uploaded since the last read:
//   - If newer: reads and returns the updated user data
//   - If unchanged: returns an empty array to signal no new data
func (f *KVStoreProvider) GetUserAttributes() ([]map[string]interface{}, error) {
	// No need to process users again if the file has not been changed since the last run
	var rawTime []byte
	if err := f.client.KV.Get(UserAttrsLastUpdatedKey, &rawTime); err != nil {
		return nil, fmt.Errorf("failed to check lastUpdated timestamp: %w", err)
	}

	// An unset key means no file has ever been uploaded, or the last one was deleted. Checked
	// explicitly because time.Time.UnmarshalJSON rejects empty input, and its error says nothing
	// about the actual cause.
	if len(rawTime) == 0 {
		return nil, fmt.Errorf("no user attributes file in the KV store: %s is unset — upload one from the System Console", UserAttrsLastUpdatedKey)
	}

	var lastUpdated time.Time
	if err := lastUpdated.UnmarshalJSON(rawTime); err != nil {
		return nil, fmt.Errorf("malformed data [%s]\nwhile checking lastUpdated timestamp: %w", rawTime, err)
	}
	// Nothing new since the last read, so there is no work to do
	if lastUpdated.IsZero() || lastUpdated.Before(f.lastSynced) {
		return []map[string]interface{}{}, nil
	}

	// Read the stored file
	var data []byte
	if err := f.client.KV.Get(UserAttrsStoreKey, &data); err != nil {
		return nil, fmt.Errorf("failed to retrieve user attrs from store: %w", err)
	}

	// Update the sync after a successful read but before validation so we dont keep reading an invalid file
	f.lastSynced = time.Now()

	// A live timestamp with no data behind it means the two keys are out of step: something wrote
	// or removed one without the other. There is nothing to sync either way, and unlike the
	// unchanged case above it is not a state the plugin produces on its own.
	if len(data) == 0 {
		return nil, fmt.Errorf("no user attributes file in the KV store: %s is set but %s is empty", UserAttrsLastUpdatedKey, UserAttrsStoreKey)
	}

	// Parse JSON
	var users []map[string]interface{}
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return users, nil
}

// Close releases any resources held by the provider.
// For KVStoreProvider, this is a no-op as no persistent resources are held — the KV store is
// reached through the shared plugin client, which the provider does not own.
func (f *KVStoreProvider) Close() error {
	return nil
}
