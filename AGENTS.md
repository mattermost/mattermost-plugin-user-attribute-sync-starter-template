# AGENTS.md

Detailed context for AI agents working on this codebase.

## What This Project Is

A **Mattermost plugin starter template** that synchronizes user attributes from an external system into Mattermost. The synced attributes appear on user profiles in the UI and are also addressable as `user.attributes.<field_name>` from attribute-based access control (ABAC) policy rules — which is why the plugin writes into the `access_control` property group. It's a reference implementation and educational resource — designed to be read, understood, and adapted. This is not a plugin that can be used as-is as a plug-and-play solution. It is expected that a developer takes this and uses it as the foundation of their own custom plugin.

**Plugin ID:** `com.mattermost.user-attribute-sync-starter-template`
**Min Mattermost version:** 11.8.0
**Languages:** Go 1.26.3+ (server), TypeScript/React (webapp)

## Architecture

```text
Plugin Activation (Once)
  ├─> Create/Update User Attribute Fields (schema)
  └─> Start Background Job (cluster-aware)

Background Job (Configurable interval, default 60min)
  ├─> Fetch Changed Values From Provider
  └─> Bulk Upsert Values via PropertyService
```

The plugin has two phases:
1. **Field sync** — Creates/updates field definitions (schema) in Mattermost on activation
2. **Value sync** — Periodically fetches user data from a provider and writes per-user values

All fields and values are stored in the `access_control` property group (`model.AccessControlPropertyGroupName`), with `ObjectType=user` and `TargetType=system`. Living in that group is what makes the fields addressable from ABAC policy expressions.

## Key Files and Their Roles

### Server (Go)

| File | Role |
|------|------|
| `server/plugin.go` | Plugin struct, OnActivate/OnDeactivate lifecycle hooks. Initializes field sync, provider, and background job. |
| `server/job.go` | Cluster-aware job scheduling via `cluster.Schedule()`. Contains `nextWaitInterval()` (calculates delay) and `runSync()` (executes sync). |
| `server/configuration.go` | Thread-safe config management with RWMutex. Settings: `SyncIntervalMinutes` (default 60). |
| `server/sync/provider.go` | `AttributeProvider` interface: `GetUserAttributes() ([]map[string]interface{}, error)` and `Close() error`. |
| `server/sync/field_sync.go` | Field definitions array and schema management. Creates/updates user attribute fields. Maintains `FieldIDCache` mapping external names to Mattermost-generated IDs. |
| `server/sync/value_sync.go` | `SyncUsers()` — matches users by email, builds PropertyValue objects, bulk upserts. Handles text, date, and multiselect value types. |
| `server/sync/file_provider.go` | Example `AttributeProvider` implementation. Reads JSON from Mattermost data directory. Tracks file modification time for incremental sync. |
| `server/main.go` | Plugin entry point (minimal). |
| `server/manifest.go` | Auto-generated from plugin.json — do not edit manually. |

### Webapp (TypeScript/React)

| File | Role |
|------|------|
| `webapp/src/index.tsx` | Plugin registration. Currently empty `initialize()` — this is a backend-focused plugin. |
| `webapp/src/manifest.ts` | Auto-generated from plugin.json — do not edit manually. |

### Build & Config

| File | Role |
|------|------|
| `plugin.json` | Plugin manifest: ID, version, executables, settings schema. |
| `Makefile` | Build orchestration (~420 lines). All build/test/deploy commands. |
| `build/setup.mk` | Extracts PLUGIN_ID, PLUGIN_VERSION, HAS_SERVER, HAS_WEBAPP from plugin.json. |
| `build/pluginctl/` | Tool for local Mattermost deployment. |
| `data/user_attributes.json` | Example data file with sample user records. |

## Field Definitions

Defined in `server/sync/field_sync.go` we have a few example user attribute field definitions in the `fieldDefinitions` array:

1. **Job Title** — `job_title`, Text type, Public access
2. **Programs** — `programs`, Multiselect type, SharedOnly access, options: Apples/Oranges/Lemons/Grapes
3. **Start Date** — `start_date`, Date type, SourceOnly access

All fields are `protected: true` (only this plugin can modify structure and write values) and `visibility: always` (shown in UI).

These fields are examples, and should be adapted to the developer's use case.

### Access Modes

- **Public**: Everyone can read all values
- **SharedOnly**: Users only see values they share with the target (multiselect only)
- **SourceOnly**: Only this plugin can read values via API

## Data Flow

1. `OnActivate()` calls `SyncFields()` which creates/updates field schema and returns a `FieldIDCache`
2. `OnActivate()` creates a `FileProvider` and starts a `cluster.Job`
3. On each job tick, `runSync()` calls `provider.GetUserAttributes()` → `SyncUsers()`
4. `SyncUsers()` iterates users, looks up each by email, calls `buildPropertyValues()` to create `PropertyValue` objects, then bulk upserts via `Property.UpsertPropertyValues()`
5. For multiselect fields, option names are translated to option IDs using `FieldIDCache`

## Extending the Plugin

### Adding a new field
Add an entry to `fieldDefinitions` in `server/sync/field_sync.go`. Restart plugin.

### Custom data source
Write code to implement the `AttributeProvider` interface (two methods: `GetUserAttributes`, `Close`). Update `plugin.go` to instantiate your provider instead of `FileProvider`.

### Field type constraint
Field types cannot be changed after creation (Mattermost limitation). Must delete and recreate.

## Build & Test Commands

```bash
make                    # Full build: check-style + test + dist
make test               # Run all tests (Go gotestsum + Node jest)
make check-style        # golangci-lint + eslint + type checking
make server             # Compile Go binaries only
make webapp             # Build webapp bundle only
make dist               # Build and create tar.gz bundle
make deploy             # Build + deploy to running Mattermost
make watch              # Auto-rebuild webapp on changes
make clean              # Remove all build artifacts
make install-go-tools   # Install golangci-lint, gotestsum
make logs-watch         # Tail plugin logs on running server
```

**Environment variables:**
- `MM_DEBUG=1` — Debug build (disables optimizations)
- `MM_SERVICESETTINGS_ENABLEDEVELOPER=1` — Build for current platform only (faster)
- `MM_SERVICESETTINGS_SITEURL` — Server URL for deploy
- `MM_ADMIN_TOKEN` — Admin token for deploy

## Testing Patterns

**Go tests** (`server/sync/*_test.go`):
- Framework: testify (assert, mock, require)
- Mocking: `plugintest.API` and `plugintest.Driver` from Mattermost
- Pattern: Table-driven tests with `t.Run()`
- Mock expectations with `.On()` and `.Return()`

**Webapp tests** (`webapp/src/*.test.tsx`):
- Framework: Jest + Enzyme
- Currently minimal (manifest test, React fragment test)

## Code Conventions

- **Error handling:** `errors.Wrap(err, "context")` / `errors.Wrapf()` from `github.com/pkg/errors`
- **Logging:** `p.client.Log.Info/Error/Warn/Debug("message", "key", value)`
- **Thread safety:** RWMutex for configuration, defensive cloning before updates
- **Graceful degradation:** Continue on partial failures; never fail entire sync for one user
- **Interface-driven:** `AttributeProvider` enables pluggable data sources

## Mattermost API Surface

Used via `pluginapi.Client`:

- `Property.GetPropertyGroup(name)` — Fetch the `access_control` property group
- `Property.GetPropertyFieldByName(groupID, objectID, fieldName)` — Lookup field by name
- `Property.CreatePropertyField(field)` — Create field (returns generated ID)
- `Property.UpdatePropertyField(groupID, field)` — Modify existing field
- `Property.UpsertPropertyValues(values)` — Bulk write user attribute values
- `User.GetByEmail(email)` — Find user by email

## Key Dependencies

**Go:**
- `github.com/mattermost/mattermost/server/public` — Plugin API, model types, cluster job scheduling
- `github.com/pkg/errors` — Error wrapping
- `github.com/stretchr/testify` — Testing

**Node:**
- React 17, Redux, mattermost-redux, @mattermost/types
- Webpack, Babel, TypeScript, ESLint, Jest
