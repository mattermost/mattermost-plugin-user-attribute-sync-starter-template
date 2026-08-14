# AGENTS.md

Detailed context for AI agents working on this codebase.

## What This Project Is

A **Mattermost plugin starter template** that synchronizes user attributes from an external system into Mattermost. The synced attributes appear on user profiles in the UI and are also addressable as `user.attributes.<field_name>` from attribute-based access control (ABAC) policy rules — which is why the plugin writes into the `access_control` property group. It's a reference implementation and educational resource — designed to be read, understood, and adapted. This is not a plugin that can be used as-is as a plug-and-play solution. It is expected that a developer takes this and uses it as the foundation of their own custom plugin.

**Plugin ID:** `com.mattermost.user-attribute-sync-starter-template`
**Min Mattermost version:** 11.9.0 (the `rank` field type requires it)
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
| `server/sync/value_sync.go` | `SyncUsers()` — matches users by email, builds PropertyValue objects, bulk upserts. Handles text, date, multiselect, and rank value types. |
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
3. **Clearance** — `clearance`, Rank type, SharedOnly access, options: CUI(1)/Confidential(2)/Secret(3)/Top Secret(4)
4. **Start Date** — `start_date`, Date type, SourceOnly access

All fields are `protected: true` (only this plugin can modify structure and write values) and `visibility: always` (shown in UI).

These fields are examples, and should be adapted to the developer's use case.

### Access Modes

- **Public**: Everyone can read all values
- **SharedOnly**: Users only see values they share with the target (select, multiselect, and rank only; on rank, a user sees their own rank and lower)
- **SourceOnly**: Only this plugin can read values via API

## Data Flow

1. `OnActivate()` calls `SyncFields()` which creates/updates field schema and returns a `FieldIDCache`
2. `OnActivate()` creates a `FileProvider` and starts a `cluster.Job`
3. On each job tick, `runSync()` calls `provider.GetUserAttributes()` → `SyncUsers()`
4. `SyncUsers()` iterates users, looks up each by email, calls `buildPropertyValues()` to create `PropertyValue` objects, then bulk upserts via `Property.UpsertPropertyValues()`
5. For fields with options (select, multiselect, rank), option names are translated to option IDs using `FieldIDCache`

**Invariants worth knowing before changing sync code:**

- `email` is the join key between external data and Mattermost users. It is consumed by `SyncUsers()` and explicitly skipped by `buildPropertyValue()`, so it is never written as an attribute. Changing the identity strategy means touching both.
- `FieldIDCache` is built once at activation and never refreshed. External names → Mattermost-generated field IDs; option names → option IDs. A cache miss on a field name is a skip-with-warning; a cache miss on an *option* name is an error for that value.
- Value JSON shape is type-dependent: text and date are marshaled strings; multiselect is an array of option **IDs**, not names; rank is a single option **ID** string, so a rank value looks like a text value on the wire and is only distinguishable by consulting the field definition.
- `fieldDefinition.hasOptions()` is the single place that decides which field types carry options, and `fieldDefinitionsByName` (a package-level index of `fieldDefinitions`) is how value sync recovers a field's declared type from the external data's key. Adding an option-bearing field type means updating `hasOptions()` and the value-formatting switch in `buildPropertyValue()` together, or values will be written as raw names instead of option IDs.
- Failure handling is per-user and per-field: unknown fields, unsupported value types, format errors, missing users, and upsert failures all log and continue. `SyncUsers()` returns `nil` unless something structural goes wrong — a "successful" sync can have written nothing.
- Timing comes from `nextWaitInterval()`, which schedules relative to `metadata.LastFinished` (0 on first run, so activation syncs immediately) and falls back to 60 minutes if the configured interval is < 1.
- `FileProvider` uses the *relative* path `data/user_attributes.json`, resolved against the Mattermost server process's working directory (i.e. `<mattermost>/data/user_attributes.json`) — not the plugin bundle. `make deploy` does not ship the file; it must be copied there separately. Its incremental behavior is mtime-based: unchanged file → empty slice → `runSync()` returns early.

## Extending the Plugin

### Adding a new field
Add an entry to `fieldDefinitions` in `server/sync/field_sync.go`. Restart plugin. Select, multiselect, and rank types also need `Options` populated; on a rank field every option needs a `Rank`, which `buildOptionsArr()` enforces.

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
make install-go-tools   # Install golangci-lint v1.64.8, gotestsum v1.7.0 into $GOBIN
make coverage           # Go coverage profile + open HTML report
make logs / logs-watch  # Fetch / tail plugin logs on running server (via build/pluginctl)
make attach             # Attach delve to the running plugin process (attach-headless, detach)
make disable / enable / reset / kill   # Plugin lifecycle on the running server
make apply              # Regenerate manifest files from plugin.json
make patch|minor|major  # Bump plugin.json version (also *-rc variants)
```

**Running a single test:**

```bash
cd server && go test ./sync/ -run TestSyncUsers -v          # one Go test
cd server && go test ./sync/ -run 'TestFileProvider_.*' -v  # pattern
cd webapp && npx jest src/manifest.test.tsx                 # one webapp test
```

`make test`/`make check-style` first run `apply`, install Go tools, and `npm install` — going through `go test` directly is much faster for iteration.

**Generated files:** `server/manifest.go` and `webapp/src/manifest.ts` are produced by `./build/bin/manifest apply` (run automatically by most Makefile targets). Edit `plugin.json`, then `make apply` — never edit the manifest files by hand. `make manifest-check` validates the manifest.

**Go module path:** `github.com/mattermost/user-attribute-sync-starter-template` (internal imports use `.../server/sync`, aliased `attrsync` in `plugin.go`). Note `.golangci.yml` still carries the upstream template's `goimports.local-prefixes`, so import grouping for local packages isn't enforced.

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
