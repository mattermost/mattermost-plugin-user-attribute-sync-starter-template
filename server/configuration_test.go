package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// closeRecorder is a no-op AttributeProvider that records whether it was closed.
type closeRecorder struct {
	closed bool
}

func (c *closeRecorder) GetUserAttributes() ([]map[string]interface{}, error) {
	return nil, nil
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

// TestNewAttributeProvider tests that each configured value maps to the right provider, that an
// unrecognized value panics rather than syncing nothing silently, and that swapping providers
// closes the one being replaced.
func TestNewAttributeProvider(t *testing.T) {
	newPlugin := func(provider string) *Plugin {
		return &Plugin{
			client:        pluginapi.NewClient(&plugintest.API{}, &plugintest.Driver{}),
			configuration: &configuration{AttributeProvider: provider},
		}
	}

	t.Run("file provider", func(t *testing.T) {
		p := newPlugin(ConfigAttributeProviderFile)
		require.IsType(t, &sync.FileProvider{}, p.NewAttributeProvider())
	})

	t.Run("kv store provider", func(t *testing.T) {
		p := newPlugin(ConfigAttributeProviderKVStore)
		require.IsType(t, &sync.KVStoreProvider{}, p.NewAttributeProvider())
	})

	t.Run("unrecognized provider", func(t *testing.T) {
		p := newPlugin("SomeOtherProvider")
		require.Panics(t, func() { p.NewAttributeProvider() })
	})

	t.Run("closes the provider it replaces", func(t *testing.T) {
		p := newPlugin(ConfigAttributeProviderKVStore)
		previous := &closeRecorder{}
		p.attributeProvider = previous

		p.NewAttributeProvider()
		assert.True(t, previous.closed, "the provider being replaced should be closed")
	})
}

// TestOnConfigurationChange tests that the config loaded from the server is applied and
// that the matching provider is built as a side effect.
func TestOnConfigurationChange(t *testing.T) {
	api := &plugintest.API{}
	defer api.AssertExpectations(t)

	// The server writes the plugin settings into the struct it is handed, so the
	// mock has to do the same rather than just return.
	api.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration")).
		Run(func(args mock.Arguments) {
			*args.Get(0).(*configuration) = configuration{
				SyncIntervalMinutes: 30,
				AttributeProvider:   ConfigAttributeProviderKVStore,
			}
		}).Return(nil).Once()

	p := &Plugin{
		MattermostPlugin: plugin.MattermostPlugin{API: api},
		client:           pluginapi.NewClient(api, &plugintest.Driver{}),
	}

	require.NoError(t, p.OnConfigurationChange())

	cfg := p.getConfiguration()
	assert.Equal(t, 30, cfg.SyncIntervalMinutes)
	assert.Equal(t, ConfigAttributeProviderKVStore, cfg.AttributeProvider)
	assert.IsType(t, &sync.KVStoreProvider{}, p.attributeProvider)
}
