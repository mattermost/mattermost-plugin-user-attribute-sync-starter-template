package main

import (
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
	attrsync "github.com/mattermost/user-attribute-sync-starter-template/server/sync"
	"github.com/pkg/errors"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// router is the HTTP router that serves API endpoints
	router *mux.Router

	// backgroundJob runs attribute sync on the configured time interval.
	backgroundJob *cluster.Job

	// fileProvider provides an example of syncing user attribute data from external source.
	fileProvider attrsync.AttributeProvider

	// groupID is the ID of the Mattermost property group this plugin reads and writes.
	// We use the "access_control" group because user attribute fields defined here can be
	// referenced from attribute-based access control (ABAC) policy rules — e.g. a channel
	// policy that only admits users whose "Programs" includes "Apples".
	groupID string

	// fieldIDCache stores mappings from external field/option names to Mattermost-generated IDs
	fieldIDCache *attrsync.FieldIDCache

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	p.initializeAPI()

	// "access_control" is the property group whose fields can be referenced
	// from attribute-based access control (ABAC) policy rules. We register
	// our user attributes here so policies can evaluate against them (e.g.
	// "only admit users whose Programs includes Apples"). Mattermost core
	// registers the group automatically on server startup; we just look it
	// up here to get the group ID we'll write fields and values against.
	group, err := p.client.Property.GetPropertyGroup(model.AccessControlPropertyGroupName)
	if err != nil {
		return errors.Wrap(err, "failed to get access_control property group")
	}
	p.groupID = group.ID

	// Sync field definitions on plugin activation and load their IDs.
	// Creates/updates the user attribute fields and stores the auto-generated
	// IDs for use during value sync.
	p.fieldIDCache, err = attrsync.SyncFields(p.client, p.groupID, manifest.Id)
	if err != nil {
		return errors.Wrap(err, "failed to sync field definitions")
	}
	p.client.Log.Info("Field sync completed successfully")

	// Initialize the file provider
	p.fileProvider = attrsync.NewFileProvider()

	// Set up the attribute sync cluster job
	// This job runs periodically to synchronize user attribute values from external
	// sources into Mattermost user attribute fields. Using cluster.Schedule ensures
	// only one server instance runs the job in multi-server deployments.
	job, err := cluster.Schedule(
		p.API,
		"AttributeSync",
		p.nextWaitInterval,
		p.runSync,
	)
	if err != nil {
		return errors.Wrap(err, "failed to schedule attribute sync job")
	}

	p.backgroundJob = job

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
// Cleans up the attribute sync cluster job and file provider to prevent orphaned resources.
func (p *Plugin) OnDeactivate() error {
	if p.backgroundJob != nil {
		if err := p.backgroundJob.Close(); err != nil {
			p.API.LogError("Failed to close attribute sync job", "err", err)
		}
	}
	if p.fileProvider != nil {
		if err := p.fileProvider.Close(); err != nil {
			p.API.LogError("Failed to close file provider", "err", err)
		}
	}
	return nil
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
