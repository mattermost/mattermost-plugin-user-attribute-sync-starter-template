package sync

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// UserAttrsStoreKey holds what the HTTP handlers in server/http_hooks.go uploaded: the file, and
// the timestamp that tells this provider the file is new.
const UserAttrsStoreKey = "user-attrs"

// StoredUserAttrs is the value under UserAttrsStoreKey.
//
// Data is the file exactly as it was uploaded, so a download returns what the admin gave us.
type StoredUserAttrs struct {
	LastUpdated time.Time `json:"lastUpdated"`
	Data        []byte    `json:"data"`
}

// ReadStoredUserAttrs reads the stored file and its timestamp.
func ReadStoredUserAttrs(client *pluginapi.Client) (StoredUserAttrs, error) {
	// Unmarshal here rather than letting KV.Get do it, so an unreachable store and a corrupt value
	// are not reported as the same error.
	var raw []byte
	if err := client.KV.Get(UserAttrsStoreKey, &raw); err != nil {
		return StoredUserAttrs{}, fmt.Errorf("failed to read %s from the KV store: %w", UserAttrsStoreKey, err)
	}

	if len(raw) == 0 {
		return StoredUserAttrs{}, nil
	}

	var stored StoredUserAttrs
	if err := json.Unmarshal(raw, &stored); err != nil {
		return StoredUserAttrs{}, fmt.Errorf("malformed data in %s: %w", UserAttrsStoreKey, err)
	}

	return stored, nil
}

// KVStoreProvider implements AttributeProvider by reading user attribute data an admin uploaded
// through the System Console, which the plugin stores in Mattermost's KV store.  THe KV Store lives in
// Mattermost's Postgres database, which is a more stable storage location than file systems in a
// cloud environment.
//
// Incremental synchronization works the similar to FileProvider's, with a stored timestamp
// standing in for the file's modification time.
type KVStoreProvider struct {
	client *pluginapi.Client

	// lastTimestampSynced is the latest file timestamp that was processed. It is in-memory only, so a
	// plugin restart re-syncs the stored file once.
	lastTimestampSynced time.Time
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
	// One read gets the file and the timestamp together, so the file is fetched even when it turns
	// out to be unchanged. Keeping it all in one key keeps key management simple; to support very
	// large files read frequently, this can be broken out into separate keys if necessary.
	stored, err := ReadStoredUserAttrs(f.client)
	if err != nil {
		return nil, err
	}

	// No file has ever been uploaded, or the last one was deleted
	if len(stored.Data) == 0 {
		return nil, fmt.Errorf("no user attributes file in the KV store: %s is unset — upload one from the System Console", UserAttrsStoreKey)
	}

	// Nothing new since the last read, so there is no work to do
	// Note we are checking if the stored file is before OR equal to the last sync, hence the !After call.
	if stored.LastUpdated.IsZero() || !stored.LastUpdated.After(f.lastTimestampSynced) {
		return []map[string]interface{}{}, nil
	}

	// Update the sync after a successful read but before validation so we dont keep reading an invalid file
	f.lastTimestampSynced = stored.LastUpdated

	// Parse JSON
	var users []map[string]interface{}
	if err := json.Unmarshal(stored.Data, &users); err != nil {
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
