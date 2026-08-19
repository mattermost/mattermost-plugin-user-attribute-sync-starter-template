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
	if lastUpdated.IsZero() || lastUpdated.Before(f.lastSynced) {
		return []map[string]interface{}{}, nil
	}

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
