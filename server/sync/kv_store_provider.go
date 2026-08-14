package sync

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost/server/public/pluginapi"
)

const UserAttrsStoreKey = "user-attrs-file"

type KVStoreProvider struct {
	client *pluginapi.Client
}

func NewKVStoreProvider(client *pluginapi.Client) *KVStoreProvider {
	return &KVStoreProvider{
		client: client,
	}
}

func (f *KVStoreProvider) GetUserAttributes() ([]map[string]interface{}, error) {
	var data []byte
	err := f.client.KV.Get(UserAttrsStoreKey, &data)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve user attrs from store: %w", err)
	}

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
