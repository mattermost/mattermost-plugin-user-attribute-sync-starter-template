package main

import (
	"errors"
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closeRecorder is a no-op AttributeProvider that records whether it was closed, and fails to
// close if closeErr is set.
type closeRecorder struct {
	closed   bool
	closeErr error
}

func (c *closeRecorder) GetUserAttributes() ([]map[string]interface{}, error) {
	return nil, nil
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return c.closeErr
}

// TestNewAttributeProvider tests that each configured value maps to the right provider, and that an
// unrecognized value panics rather than syncing nothing silently.
func TestNewAttributeProvider(t *testing.T) {
	p := &Plugin{client: pluginapi.NewClient(&plugintest.API{}, &plugintest.Driver{})}

	t.Run("file provider", func(t *testing.T) {
		require.IsType(t, &sync.FileProvider{}, p.newAttributeProvider(ConfigAttributeProviderFile))
	})

	t.Run("kv store provider", func(t *testing.T) {
		require.IsType(t, &sync.KVStoreProvider{}, p.newAttributeProvider(ConfigAttributeProviderKVStore))
	})

	t.Run("unrecognized provider", func(t *testing.T) {
		require.Panics(t, func() { p.newAttributeProvider("SomeOtherProvider") })
	})
}

// TestEnsureAttributeProvider tests the reconciliation the sync job performs on every run: build
// the configured provider once, keep it while the setting is unchanged, and close it when the
// setting names a different one.
func TestEnsureAttributeProvider(t *testing.T) {
	newPlugin := func(t *testing.T, provider string) *Plugin {
		t.Helper()

		api := &plugintest.API{}
		t.Cleanup(func() { api.AssertExpectations(t) })
		mockLogs(api)

		return &Plugin{
			client:        pluginapi.NewClient(api, &plugintest.Driver{}),
			configuration: &configuration{AttributeProvider: provider},
		}
	}

	t.Run("builds the configured provider on first use", func(t *testing.T) {
		p := newPlugin(t, ConfigAttributeProviderKVStore)
		require.Nil(t, p.attributeProvider, "nothing should have built one before the first sync")

		assert.IsType(t, &sync.KVStoreProvider{}, p.ensureAttributeProvider())
		assert.Equal(t, ConfigAttributeProviderKVStore, p.attributeProviderKind)
	})

	t.Run("reuses the provider while the setting is unchanged", func(t *testing.T) {
		p := newPlugin(t, ConfigAttributeProviderFile)

		first := p.ensureAttributeProvider()
		assert.Same(t, first, p.ensureAttributeProvider())
	})

	t.Run("closes and replaces the provider when the setting changes", func(t *testing.T) {
		p := newPlugin(t, ConfigAttributeProviderKVStore)
		previous := &closeRecorder{}
		p.attributeProvider = previous
		p.attributeProviderKind = ConfigAttributeProviderFile

		assert.IsType(t, &sync.KVStoreProvider{}, p.ensureAttributeProvider())
		assert.True(t, previous.closed, "the provider being replaced should be closed")
		assert.Equal(t, ConfigAttributeProviderKVStore, p.attributeProviderKind)
	})

	t.Run("replaces the provider even when closing it fails", func(t *testing.T) {
		p := newPlugin(t, ConfigAttributeProviderFile)
		p.attributeProvider = &closeRecorder{closeErr: errors.New("close failed")}
		p.attributeProviderKind = ConfigAttributeProviderKVStore

		assert.IsType(t, &sync.FileProvider{}, p.ensureAttributeProvider())
	})
}
