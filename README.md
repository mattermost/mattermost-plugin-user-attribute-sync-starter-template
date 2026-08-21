# User Attribute Sync Starter Template

**This is a starter template, not a production-ready plugin. It is meant to be forked and adapted to your own external data source and field definitions. Do not install it as-is expecting a working integration.**

A Mattermost plugin starter template that demonstrates how to synchronize user attributes from an external system into Mattermost. Synced attributes appear on user profiles in the UI and can also be referenced from attribute-based access control (ABAC) policy rules — this is why the plugin writes into the `access_control` property group rather than a plugin-private group. The template serves as both a working reference implementation and an educational resource for plugin developers.

## What This Template Demonstrates

Mattermost's property system lets you store structured per-user metadata. A **field** defines the schema (name, type, options), while a **value** stores the actual data for a specific user. For select, multiselect, and rank fields, **options** define the allowed choices. Fields written into the `access_control` group also become available as `user.attributes.<field_name>` inside ABAC policy expressions.

This plugin shows how to create fields with hardcoded definitions and synchronize values from external data sources. Fields are defined explicitly in code with their types (text, date, multiselect, rank), and the plugin uses Mattermost's cluster job system to run periodic synchronization tasks. The implementation includes incremental synchronization that processes only changed data after the initial sync.

The template creates four example user attribute fields that demonstrate the different access control modes and value types: Job Title (text, public access), Programs (multiselect with options, shared-only access), Clearance (rank with ordered options, shared-only access), and Start Date (date, source-only access). All fields are marked as visible in the UI and protected (only this plugin can modify structure and write values).

It also contains  **two example, interchangeable data sources**, so you can see the same sync driven from more than one kind of input. One reads a JSON file from the server's filesystem; the other reads a JSON file an admin uploads through the System Console, which the plugin keeps in Mattermost's key-value store. The second also exists in case direct filesystem access is not an option for your Mattermost deployment. Which source is used is an ordinary plugin setting, so you can switch between them on a running server. See [Choosing a Data Source](#choosing-a-data-source).

## Architecture Overview

```text
Plugin Activation (Once)
  ├─> Register HTTP Routes
  ├─> Create/Update User Attribute Fields
  ├─> Construct The Configured Data Source
  └─> Start Background Job

Background Job (On timed interval)
  ├─> Fetch Changed Values From The Configured Data Source
  │     ├── File Provider     ──> reads <mattermost>/data/user_attributes.json
  │     └── KV Store Provider ──> reads the plugin key-value store
  └─> Bulk Upsert Values

System Console (Admin, whenever they choose)
  └─> "User Attribute Source" Setting
        ├─> Pick the data source
        └─> For KV Store: upload / download / delete the stored file
              └─> Plugin HTTP API ──> plugin key-value store
```

The sync itself is unchanged by which source is selected: both sources satisfy the same interface and hand back the same shape of data.

### Key Components

- **Field Definitions** (`server/sync/field_sync.go`) - Hardcoded schema with field types and options
- **Value Sync** (`server/sync/value_sync.go`) - User attribute value synchronization
- **Provider Interface** (`server/sync/provider.go`) - The contract every data source implements
- **File Provider** (`server/sync/file_provider.go`) - Example JSON file-based data source
- **KV Store Provider** (`server/sync/kv_store_provider.go`) - Example data source fed by admin upload
- **Job Orchestrator** (`server/job.go`) - Cluster-aware periodic sync scheduler, and the selection of the configured source
- **HTTP API** (`server/http_hooks.go`) - Sysadmin-only endpoints for managing the uploaded file
- **Settings UI** (`webapp/src/components/`) - The custom System Console setting that drives both of the above

## Building from Source

### Prerequisites

- Mattermost server 11.9.0 or later
- Go 1.26.3 or later (matches the version Mattermost server pins)
- Node v20.11 — needed for the webapp bundle and the end-to-end tests

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

3. Upload the plugin through System Console → Plugin Management, or use:
   ```bash
   make deploy
   ```

4. **Important: Give the plugin some data.** Nothing syncs until you do, and how you do it depends on which source you use. The default is the file provider, so if you change nothing:

   ```bash
   cp data/user_attributes.json /path/to/mattermost/data/user_attributes.json
   ```

   The path is `data/user_attributes.json` relative to the **Mattermost server's** working directory, not the plugin directory, and `make deploy` does not put it there for you.

   To use the upload source instead, go to **System Console → Plugins → User Attribute Sync Starter Template**, set **User Attribute Source** to *Direct Upload*, click **Save**, then use **Choose File** and **Upload** in the same section to send `data/user_attributes.json` to the server. Nothing needs to be on the server's filesystem when *Direct Upload* is selected.

   Either way, update the JSON with your own users' email addresses and attributes — the file in this repository is a three-record example of the expected format.

## What to Expect

When the plugin activates, it creates the four user attribute fields (Job Title, Programs, Clearance, and Start Date) in Mattermost. These fields appear in System Console → User Attributes. If the fields already exist from a previous activation, the plugin updates them to match the hardcoded definitions.

Immediately after activation, the plugin runs its first synchronization. It reads the attributes data from whichever source is configured, matches users by email address, and populates the user attribute values for each user found. The plugin logs its progress and any errors (such as users not found in Mattermost) during this process.

After the initial sync, the plugin checks for changes every 60 minutes by default. Each source decides for itself which it is looking at: the file provider compares the modification time of `user_attributes.json`, and the KV store provider compares a timestamp the plugin records whenever an admin uploads. If a change is detected, the plugin syncs every user in the data again. You can adjust the sync interval in the plugin configuration settings.

The synced user attribute values can be viewed in System Console → User Attributes or through the Mattermost API, and they are also available to ABAC policy rules as `user.attributes.<field_name>`.

## Choosing a Data Source

The **User Attribute Source** setting in System Console → Plugins → User Attribute Sync Starter Template selects which of the two example sources the plugin reads from. Changing it takes effect on Save, without restarting the plugin.

| | Local Filesystem | Direct Upload |
|---|---|---|
| Reads from | `<mattermost>/data/user_attributes.json` | Mattermost's key-value store |
| Data is placed by | You, on the server's filesystem | An admin, through the System Console |
| Survives a container restart | No | Yes — the KV store is in the database |
| Works on Mattermost Cloud | No — no filesystem access | Yes |
| Detects changes by | File modification time | A timestamp written on upload |
| Good for | Local development; servers you have shell access to | Cloud and containerized deployments; letting an admin manage the data without server access |

Neither is meant to be the source you ship with. They are two worked examples of the same interface, chosen to be different enough to be instructive.

### Managing the Uploaded File

With *Direct Upload* selected, the settings section grows a small panel for the stored file. It tells you whether one is already present, and lets you replace it, download it back, or delete it. Files are validated before being sent — they must be a JSON array of objects and no larger than 10 MB — and rejected files never reach the server.

Downloading returns the exact file that was uploaded, which is the quickest way to confirm what the plugin is actually working from. Deleting asks for confirmation first, because there is no server-side copy to restore from.

These actions are immediate and do not go through the console's Save button, because they act on stored data rather than on a setting.

### The HTTP API Behind It

The upload panel is a client for four endpoints the plugin registers itself, in `server/http_hooks.go`. They are worth knowing about if you want to script data loading, and they are the part of the template to copy if your own plugin needs an authenticated API.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/user_attributes` | Store an attributes file. Body is the raw JSON. |
| `GET` | `/user_attributes` | Download the stored file. |
| `GET` | `/user_attributes/status` | `{"exists": bool, "lastUpdated": time\|null}` — check what is stored, and when it was uploaded, without downloading. |
| `DELETE` | `/user_attributes` | Remove the stored file. |

All four are prefixed with `/plugins/com.mattermost.user-attribute-sync-starter-template`, and all four require a **system admin** — these endpoints overwrite the data behind every user's attributes, and those attributes can gate channel access through ABAC policies. Authentication uses the `Mattermost-User-Id` header, which the Mattermost server sets on the way in and which is the only header a plugin can trust.

Uploads are validated for shape only: the payload has to be a JSON array of objects. Individual records are deliberately not checked, because rejecting an entire file over one bad record would be the wrong trade-off — external data routinely contains a few unusable records, and refusing all of it means syncing nothing. Bad records are handled one at a time during sync, where each one logs a warning and the rest continue.

```bash
# Upload with curl, as a system admin
curl -X POST \
  -H "Authorization: Bearer $MM_ADMIN_TOKEN" \
  --data-binary @data/user_attributes.json \
  http://localhost:8065/plugins/com.mattermost.user-attribute-sync-starter-template/user_attributes
```

## Access Control

This template demonstrates Mattermost's field-level access control system through its example fields, covering each of the three access modes.

### Access Modes

**Public Access** (Job Title example): Everyone can read all field values via API and UI. Best for non-sensitive organizational data like job titles, departments, or office locations.

**Source-Only Access** (Start Date example): Only this plugin can read field values via API. Other users, admins, and integrations see empty options and no values. Useful for data that must be synchronized but should remain private, like employee start dates or internal identifiers. Even users cannot see their own values through the API.

**Shared-Only Access** (Programs and Clearance examples): Users can only see field options and values they share with the target user. Only works with select, multiselect, and rank field types. Example: If Alice is in [Apples, Bananas] and Bob is in [Bananas, Oranges], Alice viewing Bob's profile only sees [Bananas] as their common program. On a rank field, a user sees their own rank and lower. Best for private categorical data where users should only discover shared attributes.

### Protected Fields

All of the example fields are marked as "protected", which means only this plugin can modify field structure (add/remove options, change types) and write values. Users and admins cannot manually edit protected fields. Read access is controlled separately by the access mode.

### UI Visibility vs Data Access

The `visibility` attribute controls whether values appear in the Mattermost UI (user profiles, user cards), but does NOT affect data access via the API. Even if visibility is set to `hidden`, data can still be retrieved via API subject to access mode permissions. To control actual data access, use the `access_mode` attribute.

### Choosing an Access Mode

Consider your data sensitivity and use case:

| Data Type | Recommended Mode | Example Fields |
|-----------|------------------|----------------|
| Public organizational info | Public | Job Title, Department, Office Location, Phone Extension |
| Sensitive internal data | Source-Only | Start Date, Salary Band, Performance Rating, Employee ID |
| Private categorical membership | Shared-Only | Programs, Projects, Teams, Certifications, Skills |
| Private ordered levels | Shared-Only | Clearance, Seniority Tier, Support Plan |

**Note**: Source-only and shared-only modes require the field to be marked as protected. Shared-only mode can only be used with select, multiselect, or rank field types.

## Customization Guide

### Adding New Fields

Edit `server/sync/field_sync.go` and add entries to the `fieldDefinitions` array:

```go
{
    Name:        "department",
    DisplayName: "Department",
    Type:        model.PropertyFieldTypeText,
    AccessMode:  model.PropertyAccessModePublic, // Choose: Public, SourceOnly, or SharedOnly
},
```

`Name` is the canonical identifier: it is the key looked up in the external data, and it is what ABAC policies reference as `user.attributes.<name>`, so it must be a valid CEL identifier (no spaces or punctuation). `DisplayName` is the free-form label shown in the UI.

Restart the plugin to create the new field. See the Access Control section above for details on access modes.

### Changing Select or Multiselect Options

Update the `Options` array in `fieldDefinitions`:

```go
{
    Name:        "programs",
    DisplayName: "Programs",
    Type:        model.PropertyFieldTypeMultiselect,
    Options: []model.CustomProfileAttributesSelectOption{
        {Name: "Apples"},
        {Name: "Oranges"},
        {Name: "Lemons"},
        {Name: "Bananas"},
    },
},
```

Mattermost generates an ID for each option, and values are stored as those IDs rather than names — the plugin reads the generated IDs back into its `FieldIDCache` and translates names to IDs when writing values.

Restart the plugin to add new options. This template plugin never removes existing options from Mattermost because users may have already selected those values.

### Adding a Rank Field

A rank field works like a select field — a user holds exactly one option — except each option also carries an integer `Rank` that defines the ordering:

```go
{
    Name:        "clearance",
    DisplayName: "Clearance",
    Type:        model.PropertyFieldTypeRank,
    Options: []model.CustomProfileAttributesSelectOption{
        {Name: "CUI", Rank: model.NewPointer(1)},
        {Name: "Confidential", Rank: model.NewPointer(2)},
        {Name: "Secret", Rank: model.NewPointer(3)},
        {Name: "Top Secret", Rank: model.NewPointer(4)},
    },
},
```

That ordering is what lets an ABAC policy express a threshold with `is at least` instead of enumerating every qualifying option — for example `user.attributes.clearance >= "Secret"` matches both Secret and Top Secret.

Every option on a rank field must have a `Rank`, since the ranks are what establish the ordering; the plugin refuses to create or update the field otherwise.

### Changing Sync Interval

The sync interval can be configured in the plugin settings. Navigate to System Console → Plugins → User Attribute Sync Starter Template and adjust the "Sync Interval (Minutes)" setting. The default is 60 minutes.

### Changing Data File Path

This applies to the *Local Filesystem* source only. Edit `server/sync/file_provider.go` and modify the constant:

```go
const defaultDataFilePath = "data/my_custom_file.json"
```

The path is resolved against the Mattermost server process's working directory. The *Direct Upload* source has no path to change — its data lives in the key-value store under the keys declared in `server/sync/kv_store_provider.go`.

### Implementing Custom Data Sources

The template ships two example providers, but the point of the interface is that you replace them with one of your own:

1. **Implement the `AttributeProvider` interface** in a new file (e.g., `server/sync/api_provider.go`):
   - `GetUserAttributes()` - Fetch user data from your external system
   - `Close()` - Clean up resources

2. **Construct it in `OnActivate`** in `server/plugin.go`:
   ```go
   p.attributeProvider = sync.NewAPIProvider(apiURL, apiKey)
   ```

3. **Handle incremental sync** by tracking state internally (e.g., last sync timestamp)

**Expect to delete the source selector.** The `AttributeProvider` setting, the provider switch in `server/job.go`, and the radios in `webapp/src/components/attribute_provider.tsx` exist so this template can demonstrate two sources in one build. A real plugin usually has exactly one, and is simpler for saying so: construct it directly in `OnActivate` and remove the setting.

If you do want a runtime choice — different sources per environment, or a migration from one to another — keep the switch, and note that the source name is spelled in five places that have to agree: the `ConfigAttributeProvider*` constants in `server/configuration.go`, the switch in `server/job.go`, the `Provider` type and radios in `attribute_provider.tsx`, the `default` in `plugin.json`, and the values in `e2e/constants.ts`. An unrecognized name panics on purpose, rather than quietly syncing nothing.

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
│   │   ├── field_sync.go         # Field creation and schema management
│   │   ├── value_sync.go         # User attribute value synchronization
│   │   ├── provider.go           # AttributeProvider interface
│   │   ├── file_provider.go      # File-based provider implementation
│   │   └── kv_store_provider.go  # Upload-based provider implementation
│   ├── plugin.go                 # Plugin lifecycle (OnActivate/OnDeactivate)
│   ├── configuration.go          # Settings
│   ├── http_hooks.go             # Sysadmin-only API for the uploaded file
│   └── job.go                    # Background job orchestration
├── webapp/src/
│   ├── index.tsx                 # Registers the custom admin console setting
│   └── components/
│       ├── attribute_provider.tsx      # The "User Attribute Source" setting
│       ├── upload_user_attributes.tsx  # Upload / download / delete panel
│       └── confirm_modal.tsx           # Confirmation dialog for deletion
├── e2e/                          # Playwright tests — see e2e/README.md
├── data/
│   └── user_attributes.json      # Example data file
└── README.md
```

### Running Tests

```bash
make test           # Run all unit tests (Go and webapp)
make check-style    # Run linting and type checking
make all            # Run check-style, test, and build
```

The end-to-end tests are separate, because they need a running Mattermost server with the plugin deployed:

```bash
make test-e2e       # Playwright, against http://localhost:8065 by default
```

See [`e2e/README.md`](e2e/README.md) for the prerequisites, how to run individual specs, and the conventions those tests follow. They are not wired into CI in this template — the right way to stand up a server for them depends on your infrastructure.

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

This is a starter template meant to be customized for your specific use case. The code is designed to be read, understood, and modified. Start by exploring the `server/sync/` directory to understand how each component works, then adapt it to your external system. If your plugin needs its own admin console setting or HTTP endpoints, `server/http_hooks.go` and `webapp/src/components/` are the parts to read next; if it does not, they are safe to delete along with the source selector.

For Mattermost plugin development questions, see the [plugin documentation](https://developers.mattermost.com/extend/plugins/).
