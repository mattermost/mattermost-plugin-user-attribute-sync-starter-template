package sync

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

const UserAttrsStoreKey = "user-attrs-file"
const UserAttrsLastUpdatedKey = "user-attrs-last-updated"

type KVStoreProvider struct {
	client     *pluginapi.Client
	lastSynced time.Time
}

func NewKVStoreProvider(client *pluginapi.Client) *KVStoreProvider {
	return &KVStoreProvider{
		client: client,
	}
}

func (f *KVStoreProvider) GetUserAttributes() ([]map[string]interface{}, error) {

	// No need to process users again if the file has not been changed since the last run
	var rawTime []byte
	if err := f.client.KV.Get(UserAttrsLastUpdatedKey, &rawTime); err != nil {
		return nil, fmt.Errorf("failed to check lastUpdated timestamp: %w", err)
	}
	// An unset key means no file has ever been uploaded, or the last one was deleted.
	// That is not an error, there is simply nothing to sync.
	if len(rawTime) == 0 {
		f.client.Log.Info("No timestamp found for last-updated")
		return []map[string]interface{}{}, nil
	}

	var lastUpdated time.Time
	if err := lastUpdated.UnmarshalJSON(rawTime); err != nil {
		return nil, fmt.Errorf("malformed data [%s]\nwhile checking lastUpdated timestamp: %w", rawTime, err)
	}
	if lastUpdated.IsZero() || lastUpdated.Before(f.lastSynced) {
		return []map[string]interface{}{}, nil
	}

	var data []byte
	if err := f.client.KV.Get(UserAttrsStoreKey, &data); err != nil {
		return nil, fmt.Errorf("failed to retrieve user attrs from store: %w", err)
	}

	// Update the sync after a successful read but before validation so we dont keep reading an invalid file
	f.lastSynced = time.Now()

	if len(data) == 0 {
		return []map[string]interface{}{}, nil
	}

	var users []map[string]interface{}
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return users, nil
}

// There are no persistent resources to close
func (f *KVStoreProvider) Close() error {
	return nil
}
