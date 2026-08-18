# End-to-end tests

Playwright tests that drive the plugin's System Console settings through a real
browser against a real Mattermost server.

## Prerequisites

These tests assume a Mattermost server that is already up and configured. They
do not start one, and this repository has no targets for doing so.

**In a `mattermost` server checkout** (not this repository — note that it also has
a `server/` directory):

1. **A running server** at `http://localhost:8065` — `cd server && make run`.
2. **The sysadmin test user** — `cd server && make test-data` creates
   `sysadmin` / `Sys@dmin-sample1`, the credentials in `constants.ts`. Any other
   system admin works; override with `MM_ADMIN_USERNAME` and `MM_ADMIN_PASSWORD`.

**In this repository:**

3. **Deploy the plugin** to that server — `make deploy` from the repo root.
4. **Install the browser** Playwright drives, once:
   ```
   cd e2e && npm install && npx playwright install chromium
   ```

No license is required, and no Docker beyond whatever the server itself needs.

## Running

```
make test-e2e                 # from the repo root
```

or, from this directory:

```
npm test                      # headless
npm run test:headed           # watch it run in a real browser
npm run test:ui               # Playwright's interactive UI mode — best for debugging
npm run test:debug            # step through with the inspector
npm run report                # open the HTML report from the last run
```

Point at a different server with `MM_SITE_URL`, and use different credentials
with `MM_ADMIN_USERNAME` / `MM_ADMIN_PASSWORD`.

## Layout

| Path | Purpose |
| --- | --- |
| `playwright.config.ts` | Timeouts, retries, worker count, reporters, trace settings |
| `constants.ts` | URLs, credentials, plugin ID, setting values |
| `utils.ts` | REST API helpers — login, enable plugin, patch settings, reset state |
| `global-setup.ts` | Runs once before all tests: logs in, saves the session, enables the plugin |
| `global-teardown.ts` | Runs once after all tests: removes the saved session file |
| `pages/` | Page objects. All locators live here, one file per page or major section |
| `tests/` | The specs |
| `assets/` | Deliberately invalid fixture files. The valid one is the repo's own `data/user_attributes.json` |

## Conventions

- **No selectors in specs.** Specs call methods on a page object in `pages/`. A
  markup change should require editing one file.
- **Locators prefer accessibility.** `getByRole` and `getByText` before
  `getByTestId`, matching the
  [Mattermost Playwright conventions](https://github.com/mattermost/mattermost/blob/master/e2e-tests/playwright/README.md).
  The single `data-testid` here is the file input, which is `display: none` and so
  has no accessible role.
- **Setup goes through the API, not the UI.** Faster, and a setup failure gives a
  clear error rather than a screenshot of a half-loaded page.
- **Behaviour, not screenshots.** These tests assert what the plugin does. Visual
  snapshot tests are possible (see `mattermost-plugin-calls` for an example) but
  they are coupled to theme, license and feature-flag state, so they are not used
  here.

## Why a single worker

`playwright.config.ts` sets `workers: 1`. Saving an admin console setting is a
read-modify-write of the server-wide `/api/v4/config` document, so parallel
workers would silently discard each other's changes. Suites large enough to need
parallelism take a filesystem lock around config-mutating tests instead;
`mattermost-plugin-calls` is the reference for that approach.

## Continuous integration

These tests are not wired into CI in this template. They need a running server
with the plugin deployed, and the right way to provide that depends on your
infrastructure. Add a job that stands up a server, runs `make deploy`, then runs
`make test-e2e`.
