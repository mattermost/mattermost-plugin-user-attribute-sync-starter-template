# AGENTS.md

Detailed context for AI agents working on this codebase.

## What This Project Is

A **Mattermost plugin starter template** that synchronizes user attributes from an external system into Mattermost. The synced attributes appear on user profiles in the UI and are also addressable as `user.attributes.<field_name>` from attribute-based access control (ABAC) policy rules — which is why the plugin writes into the `access_control` property group. It's a reference implementation and educational resource — designed to be read, understood, and adapted. This is not a plugin that can be used as-is as a plug-and-play solution. It is expected that a developer takes this and uses it as the foundation of their own custom plugin.

It ships **two example data sources**, selected by an admin at runtime: `FileProvider` (a JSON file on the server's filesystem) and `KVStoreProvider` (a JSON file uploaded through the System Console into the plugin KV store). `KVStoreProvider` exists mainly because the filesystem source is awkward or impossible to deploy in some environments — Cloud installations give no direct filesystem access, and container filesystems are ephemeral, so a file placed next to the server does not survive a restart. It also happens to demonstrate the pieces a provider needs if it wants them: a custom admin console setting, a plugin HTTP API, and server-side storage.

**Plugin ID:** `com.mattermost.user-attribute-sync-starter-template`
**Min Mattermost version:** 11.9.0 (the `rank` field type requires it)
**Languages:** Go 1.26.3+ (server), TypeScript/React (webapp)

## Architecture

```text
Plugin Activation (Once)
  ├─> Register HTTP routes (mux router on ServeHTTP)
  ├─> Create/Update User Attribute Fields (schema)
  ├─> Construct the configured AttributeProvider
  └─> Start Background Job (cluster-aware)

Background Job (Configurable interval, default 60min)
  ├─> Fetch Changed Values From The Configured Provider
  │     ├── FileProvider     — reads <mattermost>/data/user_attributes.json
  │     └── KVStoreProvider  — reads the plugin KV store
  └─> Bulk Upsert Values via PropertyService

System Console (Admin, on demand)
  └─> Custom "User Attribute Source" setting (webapp)
        ├─> Pick the provider (radio)
        └─> For KVStoreProvider: upload / download / delete the stored file
              └─> Plugin HTTP API ──> KV store
```

The plugin has two sync phases:
1. **Field sync** — Creates/updates field definitions (schema) in Mattermost on activation
2. **Value sync** — Periodically fetches user data from a provider and writes per-user values

Plus a third, admin-driven path that is not sync at all: the custom System Console setting, which chooses the provider and (for `KVStoreProvider`) manages the stored data file over the plugin's own HTTP API.

All fields and values are stored in the `access_control` property group (`model.AccessControlPropertyGroupName`), with `ObjectType=user` and `TargetType=system`. Living in that group is what makes the fields addressable from ABAC policy expressions.

## Key Files and Their Roles

### Server (Go)

| File | Role |
|------|------|
| `server/plugin.go` | Plugin struct, OnActivate/OnDeactivate lifecycle hooks. Initializes the API router, field sync, provider, and background job. |
| `server/http_hooks.go` | `ServeHTTP` + `initializeAPI()`. The `gorilla/mux` router and the four `/user_attributes` handlers, the sysadmin permission check, and the JSON response helpers. |
| `server/job.go` | Cluster-aware job scheduling via `cluster.Schedule()`. Contains `nextWaitInterval()` (calculates delay) and `runSync()` (executes sync via `p.attributeProvider`). |
| `server/configuration.go` | Thread-safe config management with RWMutex. Settings: `SyncIntervalMinutes` (default 60) and `AttributeProvider`. Also holds `NewAttributeProvider()`, the provider factory/switch. |
| `server/sync/provider.go` | `AttributeProvider` interface: `GetUserAttributes() ([]map[string]interface{}, error)` and `Close() error`. |
| `server/sync/field_sync.go` | Field definitions array and schema management. Creates/updates user attribute fields. Maintains `FieldIDCache` mapping external names to Mattermost-generated IDs. |
| `server/sync/value_sync.go` | `SyncUsers()` — matches users by email, builds PropertyValue objects, bulk upserts. Handles text, date, multiselect, and rank value types. |
| `server/sync/file_provider.go` | Example `AttributeProvider`. Reads JSON from the Mattermost data directory. Tracks file modification time for incremental sync. |
| `server/sync/kv_store_provider.go` | Example `AttributeProvider`. Reads the JSON uploaded via the HTTP API out of the plugin KV store. Owns the two KV keys (`UserAttrsStoreKey`, `UserAttrsLastUpdatedKey`) and gates work on the stored timestamp. |
| `server/main.go` | Plugin entry point (minimal). |
| `server/manifest.go` | Auto-generated from plugin.json — do not edit manually. |

### Webapp (TypeScript/React)

The webapp exists only to render the custom `AttributeProvider` setting in the System Console. There is no channel/RHS/post UI.

| File | Role |
|------|------|
| `webapp/src/index.tsx` | Plugin registration. `registerAdminConsoleCustomSetting('AttributeProvider', AttributeProvider, {showTitle: true})` — the one hook this plugin uses. |
| `webapp/src/components/attribute_provider.tsx` | The custom setting itself. Two radios (`Local Filesystem` → `FileProvider`, `Direct Upload` → `KVStore`) plus a "Source Details" panel that renders per-provider guidance. Receives `value`/`onChange`/`disabled`/`setByEnv` from the admin console. |
| `webapp/src/components/upload_user_attributes.tsx` | The Direct Upload panel. Client-side validation (10 MB cap, must parse as a JSON array of objects), then upload/download/delete against the plugin HTTP API. Probes `/user_attributes/exists` on mount to show whether a file is already stored. Takes `disabled` but deliberately not `setByEnv` — see Admin Console Setting below. |
| `webapp/src/components/confirm_modal.tsx` | Local `react-bootstrap` confirm dialog, used to gate deletion. |
| `webapp/src/components/*.scss` | Styles for the above. `webapp/src/types/scss.d.ts` declares `*.scss` so TypeScript accepts the side-effect imports. |
| `webapp/src/manifest.ts` | Auto-generated from plugin.json — do not edit manually. |

### E2E (Playwright)

`e2e/` is a standalone npm project (its own `package.json`, `tsconfig.json`, `.eslintrc.json`) driving the System Console setting through a real browser against a real server. `e2e/README.md` is the authoritative guide — prerequisites, layout, and the conventions it holds itself to (page objects only, accessibility-first locators, API-driven setup, `workers: 1` because saving admin console settings is a read-modify-write of the server-wide config).

| Path | Role |
|------|------|
| `e2e/constants.ts` | Base URL, admin credentials, plugin ID, provider values, fixture paths. Mirrors constants in `server/configuration.go` and `plugin.json`. |
| `e2e/utils.ts` | REST helpers — login, enable plugin, patch a plugin setting, upload/delete the stored file. |
| `e2e/global-setup.ts` / `global-teardown.ts` | Log in once, save the admin storage state, enable the plugin; remove the state file afterwards. |
| `e2e/pages/plugin_settings_page.ts` | The only page object. All locators for the plugin's settings section live here. |
| `e2e/tests/settings.spec.ts` | Provider radios render, and the upload controls appear only for Direct Upload. |
| `e2e/tests/user_attributes.spec.ts` | Upload/download/delete round trips, invalid-file rejection, delete-confirmation behaviour. |
| `e2e/assets/` | Deliberately invalid fixtures. The valid fixture is the repo's own `data/user_attributes.json`. |

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

1. `OnActivate()` calls `initializeAPI()` to build the HTTP router, then `SyncFields()`, which creates/updates field schema and returns a `FieldIDCache`
2. `OnActivate()` calls `NewAttributeProvider()` to construct the configured provider and starts a `cluster.Job`
3. On each job tick, `runSync()` calls `p.attributeProvider.GetUserAttributes()` → `SyncUsers()`
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
- `KVStoreProvider` is the same contract with a different "unchanged" test: it compares the `user-attrs-last-updated` KV timestamp (written by the upload handler) against its own in-memory `lastSynced`, and returns an empty slice when the stored timestamp is older. `lastSynced` is per-process and not persisted, so a plugin restart re-syncs the stored file once. It is set immediately after the read and *before* JSON parsing — deliberately, so a stored file that fails to parse is not retried on every tick.
- **Both providers distinguish "no data" from "no change", and only the second is an empty slice.** No data is an error, so it lands in the log via `runSync`'s `Failed to fetch changed users`: `FileProvider` gets there through `os.Stat` failing, `KVStoreProvider` through explicit `len()` checks on its two keys. The reasoning is that sync has been pointed at a specific place, so finding nothing there is a misconfiguration rather than a steady state — a fresh install therefore logs an error every tick until it is given data. If you add a provider, match this: return an empty slice only for "nothing changed".
- The unset-timestamp case is checked explicitly rather than left to `time.Time.UnmarshalJSON`, which rejects empty input with a message that describes neither the key nor the cause. That check is also what makes the `lastUpdated.IsZero()` branch reachable only via a literal zero timestamp in the store.
- The two KV keys are a pair. `UserAttrsStoreKey` (`user-attrs-file`) holds the raw uploaded bytes; `UserAttrsLastUpdatedKey` (`user-attrs-last-updated`) holds a JSON-marshaled `time.Time` and is what makes the provider notice a new upload. Writing the file without the timestamp means the sync never picks it up. In the handlers, the timestamp write and delete are deliberately best-effort — once the file itself has been written, no handler returns an error for a timestamp failure, it only logs.

**Provider selection:**

- `NewAttributeProvider()` in `server/configuration.go` is the switch on the `AttributeProvider` setting. It closes the previous provider before returning the new one, and **panics on an unrecognized value** — adding a provider means adding both a `Config...` constant and a `case`.
- It is called from two places: `setConfiguration()` (so changing the setting swaps the live provider without a restart) and `OnActivate()`. `setConfiguration()` runs while holding `configurationLock`, which is why `NewAttributeProvider()` reads `p.configuration` directly rather than calling `getConfiguration()` — using the accessor there would deadlock.
- The webapp radio values, the Go constants, and `plugin.json`'s `default` all have to agree on the literal strings `FileProvider` and `KVStore`. Four places encode them: `ConfigAttributeProvider*` in `server/configuration.go`, the `Provider` union in `attribute_provider.tsx`, `plugin.json`, and `e2e/constants.ts`.
- Mattermost lowercases plugin setting keys in the stored config, so the setting reads as `attributeprovider` over `/api/v4/config` even though `plugin.json` declares `AttributeProvider`. The e2e helpers rely on this.

## HTTP API

Registered in `server/http_hooks.go` on a `gorilla/mux` router, served through the `ServeHTTP` plugin hook. Full paths are prefixed `/plugins/com.mattermost.user-attribute-sync-starter-template`.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/user_attributes` | Upload the attributes file into the KV store. Body is the raw JSON file. |
| `GET` | `/user_attributes` | Download the stored file verbatim. `404` when nothing is stored. |
| `GET` | `/user_attributes/exists` | `{"exists": bool}` — lets the UI show whether a file is already stored without downloading it. |
| `DELETE` | `/user_attributes` | Remove the stored file and its timestamp. |

Things to preserve when changing these:

- **The sysadmin check is router middleware, not per-handler.** `initializeAPI` applies `requireSysadmin` via `router.Use`, so a route added later is protected by default rather than depending on the author remembering. It reads the `Mattermost-User-Id` header — the only header the plugin can trust, because the Mattermost server sets it on the way in and strips any client-supplied value — and requires `model.PermissionManageSystem`. Two consequences: the whole router is admin-only, so a genuinely public route needs the protected routes moved to a subrouter with `Use` applied there; and gorilla/mux builds the middleware chain only on a matched route (`Router.Match`), so unknown paths 404 without reaching the check.
- **Uploads are bounded twice.** `http.MaxBytesReader` caps the body at `maxFileSizeBytes` (10 MB) server-side, and the UI checks `MAX_FILE_BYES` (also 10 MB) before sending. The two constants are independent — keep them in step.
- **Validation is shape-only, on both sides, by design.** Server and client both require the payload to unmarshal as an array of objects; neither validates emails, field names, or value types. This is not a gap to close: external data routinely contains a few unusable records, and rejecting the whole file over one of them would mean syncing nothing. Per-record failures are already handled where they belong, in value sync, which logs and continues (see the failure-handling invariant above). Adding record-level validation to the upload endpoint would move that decision to the worst possible place — an all-or-nothing gate in front of otherwise good data.
- The webapp reaches these routes with plain `fetch`, using `Client4.getOptions()` on `POST`/`DELETE` to pick up the CSRF token. `GET`s need no options.

## Admin Console Setting

`plugin.json` declares `AttributeProvider` as `"type": "custom"`, which tells the System Console to render a webapp-supplied component instead of a built-in control. `index.tsx` supplies it via `registry.registerAdminConsoleCustomSetting`.

- The console passes considerably more props than this component declares (`label`, `helpText`, `config`, `license`, `registerSaveAction`, `setSaveNeeded`, `showConfirm`, …). `attribute_provider.tsx` deliberately takes only `id`, `value`, `onChange`, `disabled`, and `setByEnv`; the rest are available if a future version needs them (see `schema_admin_settings.tsx`'s `buildCustomSetting` in the server repo).
- `onChange(id, value)` is what marks the setting dirty. The console still owns the Save button, so **provider changes only take effect once the admin saves**, which is what triggers `OnConfigurationChange` → `setConfiguration` → provider swap. The upload/download/delete buttons are the exception: they hit the plugin API directly and take effect immediately, with no Save.
- **`setByEnv` applies to the radios, not to the file controls.** It means the *setting* is pinned by an environment variable, which is a reason not to let an admin pick a different source (matching the console's own `RadioSetting`, which does `disabled || setByEnv`). It says nothing about the stored file: that lives in the KV store, and no environment variable can put it there. Pinning the source to `KVStore` is in fact the case where uploading is the only way to give the plugin data, so `AttributeProvider` deliberately does not pass `setByEnv` down to `UploadUserAttributes`. `disabled` — plugin off, or a read-only admin — does gate both.
- Because `showTitle: true` is passed at registration, the console wraps the component in its own `Setting` (rendering `display_name` as the label and `help_text` beneath it). The `help_text` was dropped from `plugin.json` because the component's "Source Details" panel already explains each option; add it back rather than duplicating text if that changes.
- If the plugin is disabled, the webapp component is not registered, and the console renders a warning banner ("In order to view this setting, enable the plugin and click Save") in place of the setting. A blank-looking setting usually means the bundle failed to load, not that the component is broken.

## Extending the Plugin

### Adding a new field
Add an entry to `fieldDefinitions` in `server/sync/field_sync.go`. Restart plugin. Select, multiselect, and rank types also need `Options` populated; on a rank field every option needs a `Rank`, which `buildOptionsArr()` enforces.

### Custom data source
Write code to implement the `AttributeProvider` interface (two methods: `GetUserAttributes`, `Close`). `FileProvider` and `KVStoreProvider` are the two worked examples — copy whichever is closer.

**The expected path for a developer adapting this template is to delete both example providers and the selection machinery, and construct their single real provider directly in `OnActivate()`.** The `AttributeProvider` setting exists so the template can demonstrate two sources side by side, not because the interface requires a choice. Runtime selection remains available if a real deployment genuinely needs it — e.g. differing sources per environment, or a migration between sources — and in that case four places have to agree on the same literal string: a `ConfigAttributeProvider*` constant and a `case` in `NewAttributeProvider()` (`server/configuration.go`), the `Provider` union and a radio in `attribute_provider.tsx`, and the values in `e2e/constants.ts`. Miss the `case` and `NewAttributeProvider()` panics; miss the radio and the value is unreachable from the console.

Whatever the source, `GetUserAttributes()` is expected to return an **empty slice when nothing has changed** — the job runs on a timer and `runSync()` returns early on an empty result. `FileProvider` decides this from file mtime, `KVStoreProvider` from a stored timestamp.

### Field type constraint
Field types cannot be changed after creation (Mattermost limitation). Must delete and recreate.

## Build & Test Commands

```bash
make                    # Full build: check-style + test + dist
make test               # Run all tests (Go gotestsum + Node jest). Does NOT run e2e.
make test-e2e           # Playwright e2e — needs a running server with the plugin deployed
make check-style        # golangci-lint + eslint + type checking, for webapp AND e2e
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
cd server && go test . -run 'TestHandle.*' -v               # the HTTP handler tests (package main)
cd webapp && npx jest src/manifest.test.tsx                 # one webapp test
cd e2e && npm test -- tests/settings.spec.ts                # one Playwright spec
cd e2e && npm test -- -g 'renders both attribute provider'  # by title
```

`make test`/`make check-style` first run `apply`, install Go tools, and `npm install` — going through `go test` directly is much faster for iteration.

`check-style` now also lints and type-checks `e2e/` (and so depends on `e2e/node_modules`), but that happens inside the `HAS_WEBAPP` guard — a fork that removes the webapp silently stops linting the e2e tests too. `make test-e2e` is separate from `make test` and from CI, because it needs a live server; see `e2e/README.md`.

**Generated files:** `server/manifest.go` and `webapp/src/manifest.ts` are produced by `./build/bin/manifest apply` (run automatically by most Makefile targets). Edit `plugin.json`, then `make apply` — never edit the manifest files by hand. `make manifest-check` validates the manifest.

**Go module path:** `github.com/mattermost/user-attribute-sync-starter-template` (internal imports use `.../server/sync`, aliased `attrsync` in `plugin.go`). Note `.golangci.yml` still carries the upstream template's `goimports.local-prefixes`, so import grouping for local packages isn't enforced.

**Environment variables:**
- `MM_DEBUG=1` — Debug build (disables optimizations)
- `MM_SERVICESETTINGS_ENABLEDEVELOPER=1` — Build for current platform only (faster)
- `MM_SERVICESETTINGS_SITEURL` — Server URL for deploy
- `MM_ADMIN_TOKEN` — Admin token for deploy
- `MM_SITE_URL` — Server the e2e tests target (default `http://localhost:8065`)
- `MM_ADMIN_USERNAME` / `MM_ADMIN_PASSWORD` — e2e admin credentials (default `sysadmin` / `Sys@dmin-sample1`, what `make test-data` creates in a server checkout)

## Testing Patterns

**Go tests** (`server/*_test.go`, `server/sync/*_test.go`):
- Framework: testify (assert, mock, require)
- Mocking: `plugintest.API` and `plugintest.Driver` from Mattermost
- Pattern: Table-driven tests with `t.Run()`
- Mock expectations with `.On()` and `.Return()`
- Each file has a `newTest*` helper that builds the subject against a fresh `plugintest.API` and registers `t.Cleanup(func() { api.AssertExpectations(t) })`, so unmet expectations fail the test automatically. Follow that shape for new tests.
- `http_hooks_test.go` drives handlers through `p.ServeHTTP` with `httptest`, the same way the server does, rather than calling handler functions directly — so route registration and the permission check are covered too. It sets the `Mattermost-User-Id` header to simulate an authenticated request and mocks `HasPermissionTo` to control authorization.
- `configuration_test.go` covers `NewAttributeProvider()`, including that it closes the previous provider (via a local `closeRecorder` stub) and panics on an unknown value.

**Webapp tests** (`webapp/src/**/*.test.tsx`):
- Framework: **Jest 29 + React Testing Library**, matching the majority of Mattermost plugins (calls, github, gitlab, jira, zoom). Query by role, assert on what the admin can see and do; `@testing-library/jest-dom` matchers are registered globally in `tests/setup.tsx`.
- **RTL is pinned to 12.1.5, and cannot be upgraded while this plugin is on React 17.** RTL 13+ declares `peerDependencies: react >=18`. Every RTL-using Mattermost plugin is on React 18 and therefore on RTL 14.x — do not copy their pin without moving React first.
- Enzyme was removed (`enzyme`, `@types/enzyme`, `enzyme-adapter-react-17-updated`, `enzyme-to-json`). It had been an unconfigured devDependency since the initial commit, inherited from an older generation of the upstream starter template, which has since dropped it too. The `snapshotSerializers` entry went with it.
- **Two jest resolution problems had to be solved before any component test could run**, and both are worth understanding before adding tests:
  - `mattermost-redux` (and `@mattermost/client` beneath it) expose subpaths via the `exports` field. Jest 27 predates `exports` support, so `mattermost-redux/client` was unresolvable and *nothing* importing `Client4` could be tested. **Upgrading to jest 29 fixed this natively** — no `moduleNameMapper` needed, which is why the other plugins' configs have no entry for it. This is what jest 27 was costing.
  - `react-bootstrap` is a **webpack external** — the host webapp supplies it at runtime, so it is deliberately not installed and jest cannot resolve it. `tests/react_bootstrap_mock.tsx` stands in for it via `moduleNameMapper`, implementing only the `Modal` surface `confirm_modal.tsx` uses. Any future external in `webpack.config.js` that is not also a real dependency needs the same treatment.
  - **Installing react-bootstrap as a devDependency instead was tried and rejected**, and not for bundle-size reasons — `externals` excludes it by configuration, so an install would cost the bundle nothing. It was rejected because (a) Mattermost's webapp uses its own fork, `github:mattermost/react-bootstrap#05559f4c`, so an upstream install would test against a different library than production; and (b) upstream `0.32.4` — the line matching that fork and the pinned `@types/react-bootstrap@0.32.37` — declares `peer react: >=15.3.0`, which npm satisfies with react-dom 19 and then fails against react 17. `--legacy-peer-deps` does install it, but removes `react-dom` from the tree and breaks RTL; the alternative is an `overrides` pin, i.e. exactly the cruft removed with enzyme. The real Modal is covered in `e2e/` against the host's actual fork.
- `attribute_provider.test.tsx` renders the **real** component tree, so the assertions are behavioural rather than prop-level. The mount-time `/user_attributes/exists` request is stubbed with a `fetch` mock; `renderSetting` awaits an `act()` flush so the resulting state update does not land after the test ends.
- The `setByEnv` case is a regression test, and it was **verified by reintroducing the bug and watching it fail**. Do the same for any regression test added here — an earlier prop-level version of this test passed even with the bug present, because the defect was in the child's own `disabled || setByEnv`.
- `react_fragment.test.tsx` was deleted: it asserted `React.version === '17.0.2'`, so it tested nothing about this plugin while guaranteeing a failure on any React upgrade. `manifest.test.tsx` is kept as a smoke test that manifest generation ran.

**E2E tests** (`e2e/tests/*.spec.ts`):
- Framework: Playwright, `workers: 1`, no server started for you
- Read `e2e/README.md` before touching these. The conventions that matter: locators live only in `pages/`, `getByRole`/`getByText` are preferred over `getByTestId`, and per-test setup goes through the REST helpers in `utils.ts` rather than the UI.

## Code Conventions

- **Error handling:** `errors.Wrap(err, "context")` / `errors.Wrapf()` from `github.com/pkg/errors`
- **Logging:** `p.client.Log.Info/Error/Warn/Debug("message", "key", value)`
- **Thread safety:** RWMutex for configuration, defensive cloning before updates
- **Graceful degradation:** Continue on partial failures; never fail entire sync for one user
- **Interface-driven:** `AttributeProvider` enables pluggable data sources
- **HTTP responses:** JSON via the `errorWithJSON` / `responseWithJSON` helpers in `http_hooks.go`, not hand-written `w.Write`. Download is the deliberate exception — it streams the stored bytes verbatim.
- **Webapp:** function components with hooks, no Redux (the admin console owns the setting's value), literal strings in JSX braces per the Mattermost eslint config, and no i18n — this is a template, so strings are inline rather than translated.
- **No license headers.** Do not add `// Copyright (c) …` / `// See LICENSE.txt …` blocks to any file. No source file in this repo carries one, and `webapp/.eslintrc.json` sets `"header/header": "off"`. Licensing lives in `LICENSE` at the repo root.

## Mattermost API Surface

Used via `pluginapi.Client`:

- `Property.GetPropertyGroup(name)` — Fetch the `access_control` property group
- `Property.GetPropertyFieldByName(groupID, objectID, fieldName)` — Lookup field by name
- `Property.CreatePropertyField(field)` — Create field (returns generated ID)
- `Property.UpdatePropertyField(groupID, field)` — Modify existing field
- `Property.UpsertPropertyValues(values)` — Bulk write user attribute values
- `User.GetByEmail(email)` — Find user by email
- `User.HasPermissionTo(userID, permission)` — The sysadmin gate on every HTTP route
- `KV.Set/Get/Delete(key, …)` — Storage behind `KVStoreProvider` and the upload API. Note `KV.Set` returns `(bool, error)`: a `false` with no error means the write did not happen, and callers here treat that as a failure.
- `Log.Info/Warn/Error/Debug` — Structured logging

Plugin hooks implemented: `OnActivate`, `OnDeactivate`, `OnConfigurationChange`, `ServeHTTP`.

## Key Dependencies

**Go:**
- `github.com/mattermost/mattermost/server/public` — Plugin API, model types, cluster job scheduling
- `github.com/gorilla/mux` — HTTP routing behind `ServeHTTP`
- `github.com/pkg/errors` — Error wrapping (note `http_hooks.go` and `kv_store_provider.go` use stdlib `errors`/`fmt.Errorf` with `%w` instead)
- `github.com/stretchr/testify` — Testing

**Node:**
- React 17, Redux, mattermost-redux, @mattermost/types
- `react-bootstrap` — used only by `confirm_modal.tsx`. It is a **webpack external** (`webpack.config.js` maps it to the host's `ReactBootstrap` global, like `react` and `redux`), so it is not installed; only `@types/react-bootstrap` is a devDependency, and it exists purely so `tsc` can resolve the import. Adding the real package would not change the bundle, but see the testing notes above for why it is still not worth doing
- Webpack, Babel, TypeScript, ESLint, Jest
- `e2e/` has its own tree: `@playwright/test`, TypeScript, ESLint + `@typescript-eslint`
