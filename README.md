# User Attribute Sync Starter Template

**This is a starter template, not a production-ready plugin. It is meant to be forked and adapted to your own external data source and field definitions. Do not install it as-is expecting a working integration.**

A Mattermost plugin starter template that demonstrates how to synchronize user attributes from an external system into Mattermost. Synced attributes appear on user profiles in the UI and can also be referenced from attribute-based access control (ABAC) policy rules — this is why the plugin writes into the `access_control` property group rather than a plugin-private group. The template serves as both a working reference implementation and an educational resource for plugin developers.

## What This Template Demonstrates

Mattermost's property system lets you store structured per-user metadata. A **field** defines the schema (name, type, options), while a **value** stores the actual data for a specific user. For multiselect fields, **options** define the allowed choices. Fields written into the `access_control` group also become available as `user.attributes.<field_name>` inside ABAC policy expressions.

This plugin shows how to create fields with hardcoded definitions and synchronize values from external data sources. Fields are defined explicitly in code with their types (text, date, multiselect), and the plugin uses Mattermost's cluster job system to run periodic synchronization tasks. The implementation includes incremental synchronization that processes only changed data after the initial sync.

The template creates three example user attribute fields that demonstrate different access control modes: Job Title (text, public access), Programs (multiselect with options, shared-only access), and Start Date (date, source-only access). All fields are marked as visible in the UI and protected (only this plugin can modify structure and write values).

## Architecture Overview

```
Plugin Activation (Once)
  ├─> Create/Update User Attribute Fields
  └─> Start Background Job

Background Job (On timed interval)
  ├─> Fetch Changed Values From External Source
  └─> Bulk Upsert Values
```

### Key Components

- **Field Definitions** (`server/sync/field_sync.go`) - Hardcoded schema with field types and options
- **Value Sync** (`server/sync/value_sync.go`) - User attribute value synchronization
- **File Provider** (`server/sync/file_provider.go`) - Example JSON file-based data source
- **Job Orchestrator** (`server/job.go`) - Cluster-aware periodic sync scheduler

## Building from Source

### Prerequisites

- Mattermost server 11.8.0 or later
- Go 1.26.3 or later (matches the version Mattermost server pins)
- Node v16 and npm v8 (if modifying webapp)

### Installation

1. Clone this repository:
   ```bash
   git clone https://github.com/mattermost/mattermost-plugin-user-attribute-sync-starter-template
   cd mattermost-plugin-user-attribute-sync-starter-template
   ```

2. Build the plugin:
   ```bash
   make
   ```

3. **Important**: Copy the example data file to your Mattermost data directory:
   ```bash
   cp data/user_attributes.json /path/to/mattermost/data/user_attributes.json
   ```

   The plugin reads from `data/user_attributes.json` relative to the Mattermost data directory (not the plugin directory). Update the JSON file with your users' email addresses and attributes. See `data/user_attributes.json` in this repository for the expected format.

4. Upload the plugin through System Console → Plugin Management, or use:
   ```bash
   make deploy
   ```

## What to Expect

When the plugin activates, it creates the three user attribute fields (Job Title, Programs, and Start Date) in Mattermost. These fields appear in System Console → User Attributes. If the fields already exist from a previous activation, the plugin updates them to match the hardcoded definitions.

Immediately after activation, the plugin runs its first synchronization. It reads the `user_attributes.json` file from the Mattermost data directory, matches users by email address, and populates the user attribute values for each user found in the data file. The plugin logs its progress and any errors (such as users not found in Mattermost) during this process.

After the initial sync, the plugin checks for changes every 60 minutes by default. The file provider tracks the modification time of `user_attributes.json` and only processes the file if it has been modified since the last sync. When changes are detected, the plugin syncs all users in the file again. You can adjust the sync interval in the plugin configuration settings.

The synced user attribute values can be viewed in System Console → User Attributes or through the Mattermost API, and they are also available to ABAC policy rules as `user.attributes.<field_name>`.

## Access Control

This template demonstrates Mattermost's field-level access control system through three example fields, each using a different access mode.

### Access Modes

**Public Access** (Job Title example): Everyone can read all field values via API and UI. Best for non-sensitive organizational data like job titles, departments, or office locations.

**Source-Only Access** (Start Date example): Only this plugin can read field values via API. Other users, admins, and integrations see empty options and no values. Useful for data that must be synchronized but should remain private, like employee start dates or internal identifiers. Even users cannot see their own values through the API.

**Shared-Only Access** (Programs example): Users can only see field options and values they share with the target user. Only works with select and multiselect field types. Example: If Alice is in [Apples, Bananas] and Bob is in [Bananas, Oranges], Alice viewing Bob's profile only sees [Bananas] as their common program. Best for private categorical data where users should only discover shared attributes.

### Protected Fields

All three example fields are marked as "protected", which means only this plugin can modify field structure (add/remove options, change types) and write values. Users and admins cannot manually edit protected fields. Read access is controlled separately by the access mode.

### UI Visibility vs Data Access

The `visibility` attribute controls whether values appear in the Mattermost UI (user profiles, user cards), but does NOT affect data access via the API. Even if visibility is set to `hidden`, data can still be retrieved via API subject to access mode permissions. To control actual data access, use the `access_mode` attribute.

### Choosing an Access Mode

Consider your data sensitivity and use case:

| Data Type | Recommended Mode | Example Fields |
|-----------|------------------|----------------|
| Public organizational info | Public | Job Title, Department, Office Location, Phone Extension |
| Sensitive internal data | Source-Only | Start Date, Salary Band, Performance Rating, Employee ID |
| Private categorical membership | Shared-Only | Programs, Projects, Teams, Certifications, Skills |

**Note**: Source-only and shared-only modes require the field to be marked as protected. Shared-only mode can only be used with select or multiselect field types.

## Customization Guide

### Adding New Fields

Edit `server/sync/field_sync.go` and add entries to the `fieldDefinitions` array:

```go
{
    Name:         "Department",
    ExternalName: "department",
    Type:         model.PropertyFieldTypeText,
    AccessMode:   model.PropertyAccessModePublic, // Choose: Public, SourceOnly, or SharedOnly
},
```

Restart the plugin to create the new field. See the Access Control section above for details on access modes.

### Changing Multiselect Options

Update the `OptionNames` array in `fieldDefinitions`:

```go
{
    Name:         "Programs",
    ExternalName: "programs",
    Type:         model.PropertyFieldTypeMultiselect,
    OptionNames:  []string{"Apples", "Oranges", "Lemons", "Bananas"},
},
```

Restart the plugin to add new options. This template plugin never removes existing options from Mattermost because users may have already selected those values.

### Changing Sync Interval

The sync interval can be configured in the plugin settings. Navigate to System Console → Plugins → User Attribute Sync Starter Template and adjust the "Sync Interval (Minutes)" setting. The default is 60 minutes.

### Changing Data File Path

Edit `server/sync/file_provider.go` and modify the constant:

```go
const defaultDataFilePath = "data/my_custom_file.json"
```

### Implementing Custom Data Sources

The template uses a file-based provider, but you can swap this for any data source:

1. **Implement the `AttributeProvider` interface** in a new file (e.g., `server/sync/api_provider.go`):
   - `GetUserAttributes()` - Fetch user data from your external system
   - `Close()` - Clean up resources

2. **Update `server/job.go`** to use your provider:
   ```go
   provider := sync.NewAPIProvider(apiURL, apiKey)
   ```

3. **Handle incremental sync** by tracking state internally (e.g., last sync timestamp)

Common provider implementations:
- **REST API**: Poll external API for changed users since last sync
- **LDAP**: Query directory for users modified after last sync time
- **Database**: Query users table with `updated_at > last_sync`
- **Webhook**: Accept push notifications of changed users (requires API endpoint)

### Field Type Constraints

**Important**: Field types cannot be changed after creation (Mattermost platform limitation). To change a field type:
1. Delete the field (all user values will be lost). You can do this via the Mattermost API or by adding code to delete the field during plugin activation.
2. Update the field definition in code
3. Restart the plugin to recreate with new type

## Development

### Project Structure

```
.
├── server/
│   ├── sync/
│   │   ├── field_sync.go       # Field creation and schema management
│   │   ├── value_sync.go       # User attribute value synchronization
│   │   ├── provider.go         # AttributeProvider interface
│   │   └── file_provider.go    # File-based provider implementation
│   ├── plugin.go               # Plugin lifecycle (OnActivate/OnDeactivate)
│   └── job.go                  # Background job orchestration
├── data/
│   └── user_attributes.json    # Example data file
└── README.md
```

### Running Tests

```bash
make test           # Run all unit tests
make check-style    # Run linting
make all            # Run check-style, test, and build
```

### Local Development

Enable local mode in your Mattermost server configuration:

```json
{
    "ServiceSettings": {
        "EnableLocalMode": true,
        "LocalModeSocketLocation": "/var/tmp/mattermost_local.socket"
    }
}
```

Then deploy automatically on changes:

```bash
make deploy
```

For continuous deployment during development:

```bash
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=your_token_here
make watch
```

## License

See LICENSE file for details.

## Questions or Issues?

This is a starter template meant to be customized for your specific use case. The code is designed to be read, understood, and modified. Start by exploring the `server/sync/` directory to understand how each component works, then adapt it to your external system.

For Mattermost plugin development questions, see the [plugin documentation](https://developers.mattermost.com/extend/plugins/).
