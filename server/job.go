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

// runSync executes the user attribute value synchronization workflow.
//
// This function runs periodically (at the interval configured in plugin settings) to synchronize
// user attribute values from external sources into Mattermost Custom Profile Attributes.
//
// Note: Field schema synchronization (creating/updating PropertyFields) happens
// once during plugin activation in OnActivate().
func (p *Plugin) runSync() {
	p.client.Log.Info("Sync starting")

	// Fetch changed users since last sync
	users, err := p.fileProvider.GetUserAttributes()
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
	err = sync.SyncUsers(p.client, p.cpaGroupID, users, p.fieldIDCache)
	if err != nil {
		p.client.Log.Error("Failed to sync user values", "error", err.Error())
		return
	}

	p.client.Log.Info("Sync completed successfully", "users_processed", len(users))
}
