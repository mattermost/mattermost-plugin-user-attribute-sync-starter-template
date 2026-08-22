package main

import (
	"time"

	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	"github.com/mattermost/user-attribute-sync-starter-template/server/sync"
)

// nextWaitInterval calculates the duration to wait before the next sync execution.
// This function is called by the cluster job scheduler to determine when to run
// the sync job next.
//
// On the first run (when metadata.LastFinished is zero), the job runs immediately.
// On subsequent runs, it waits for the configured interval from the last completion time.
func (p *Plugin) nextWaitInterval(now time.Time, metadata cluster.JobMetadata) time.Duration {
	// Get the configured sync interval (defaults to 60 minutes if not set)
	config := p.getConfiguration()
	syncIntervalMinutes := config.SyncIntervalMinutes
	if syncIntervalMinutes < 1 {
		syncIntervalMinutes = 60 // Fallback to default if invalid
	}

	// First run - execute immediately
	if metadata.LastFinished.IsZero() {
		return 0
	}

	// Calculate next scheduled run time
	nextRunTime := metadata.LastFinished.Add(time.Duration(syncIntervalMinutes) * time.Minute)

	// If next run time is in the past, run immediately
	if nextRunTime.Before(now) {
		return 0
	}

	// Return duration until next scheduled run
	return nextRunTime.Sub(now)
}

// ensureAttributeProvider returns the provider based on plugin configuration, building it on
// first use and replacing it when the setting has changed since.
//
// This is only called in runSync to avoid concurrency issues of closing a provider while it is
// still being read.
func (p *Plugin) ensureAttributeProvider() sync.AttributeProvider {
	kind := p.getConfiguration().AttributeProvider

	if p.attributeProvider != nil && p.attributeProviderKind == kind {
		return p.attributeProvider
	}

	if p.attributeProvider != nil {
		// Replace it even if closing failed; syncing from the wrong source is worse than leaking.
		if err := p.attributeProvider.Close(); err != nil {
			p.client.Log.Error("Failed to close the replaced attribute provider", "err", err)
		}
	}

	p.attributeProvider = p.newAttributeProvider(kind)
	p.attributeProviderKind = kind

	return p.attributeProvider
}

// newAttributeProvider constructs the data source named by kind.
//
// It panics on an unrecognized value: the setting is constrained to the ConfigAttributeProvider*
// values by the System Console, so anything else means the switch and the constants have drifted
// apart, and syncing nothing silently would be worse than failing loudly.
func (p *Plugin) newAttributeProvider(kind string) sync.AttributeProvider {
	switch kind {
	case ConfigAttributeProviderFile:
		return sync.NewFileProvider()
	case ConfigAttributeProviderKVStore:
		return sync.NewKVStoreProvider(p.client)
	default:
		panic("unrecognized Attribute Provider")
	}
}

// runSync executes the user attribute value synchronization workflow.
//
// This function runs periodically (at the interval configured in plugin settings) to synchronize
// user attribute values from external sources into Mattermost user attribute fields.
//
// Note: Field schema synchronization (creating/updating PropertyFields) happens
// once during plugin activation in OnActivate().
func (p *Plugin) runSync() {
	p.client.Log.Info("Sync starting")

	provider := p.ensureAttributeProvider()

	// Fetch changed users since last sync
	users, err := provider.GetUserAttributes()
	if err != nil {
		p.client.Log.Error("Failed to fetch changed users", "error", err.Error())
		return
	}

	if len(users) == 0 {
		p.client.Log.Info("No changed users to sync")
		return
	}

	p.client.Log.Info("Fetched users for sync", "count", len(users))

	// Sync user values using the field ID cache loaded during activation
	err = sync.SyncUsers(p.client, p.groupID, users, p.fieldIDCache)
	if err != nil {
		p.client.Log.Error("Failed to sync user values", "error", err.Error())
		return
	}

	p.client.Log.Info("Sync completed successfully", "users_processed", len(users))
}
