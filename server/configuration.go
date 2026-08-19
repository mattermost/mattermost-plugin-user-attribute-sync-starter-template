package main

import (
	"reflect"

	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
	"github.com/pkg/errors"
)

// The accepted values of the AttributeProvider setting. These strings are stored in the server
// configuration, so they are effectively a wire format: the radio values in
// webapp/src/components/attribute_provider.tsx and the default in plugin.json have to match them
// exactly, and renaming one means migrating existing installations.
const (
	ConfigAttributeProviderFile    = "FileProvider"
	ConfigAttributeProviderKVStore = "KVStore"
)

// configuration captures the plugin's external configuration as exposed in the Mattermost server
// configuration, as well as values computed from the configuration. Any public fields will be
// deserialized from the Mattermost server configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// configuration can change at any time, access to the configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.

type configuration struct {
	// SyncIntervalMinutes determines how often (in minutes) the plugin syncs user attributes
	// from the external source. Must be at least 1 minute.
	SyncIntervalMinutes int

	// AttributeProvider selects which example data source to sync from — one of the
	// ConfigAttributeProvider* values above. It is rendered in the System Console by a custom
	// webapp component rather than a built-in control, because the KVStore option needs upload
	// and download buttons alongside the choice itself.
	//
	// A plugin built from this template would normally have a single data source and no such
	// setting; it exists here to demonstrate both examples in one build.
	AttributeProvider string
}

// Clone shallow copies the configuration. Your implementation may require a deep copy if
// your configuration has reference types.
func (c *configuration) Clone() *configuration {
	var clone = *c
	return &clone
}

// getConfiguration retrieves the active configuration under lock, making it safe to use
// concurrently. The active configuration may change underneath the client of this method, but
// the struct returned by this API call is considered immutable.
func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
//
// It also rebuilds the attribute provider, so changing the source in the System Console takes
// effect without restarting the plugin.
func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
	p.attributeProvider = p.NewAttributeProvider()
}

// NewAttributeProvider constructs the data source named by the current configuration, closing
// whichever provider was in use before so its resources are not leaked.
//
// It reads p.configuration directly rather than calling getConfiguration(), because
// setConfiguration calls it while already holding configurationLock and RWMutex is not reentrant.
//
// It panics on an unrecognized value: the setting is constrained to the ConfigAttributeProvider*
// values by the System Console, so anything else means the switch and the constants have drifted
// apart, and syncing nothing silently would be worse than failing loudly.
//
// A plugin adapted from this template would usually drop this switch and construct its one real
// provider in OnActivate instead.
func (p *Plugin) NewAttributeProvider() sync.AttributeProvider {
	if p.attributeProvider != nil {
		p.attributeProvider.Close()
	}

	switch p.configuration.AttributeProvider {
	case ConfigAttributeProviderFile:
		return sync.NewFileProvider()
	case ConfigAttributeProviderKVStore:
		return sync.NewKVStoreProvider(p.client)
	default:
		panic("unrecognized Attribute Provider")
	}
}

// OnConfigurationChange is invoked when configuration changes may have been made.
func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	p.setConfiguration(configuration)

	return nil
}
