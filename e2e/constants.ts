/* eslint-disable no-process-env */

// The server under test.
//
// MM_SITE_URL is the conventional override across Mattermost's Playwright
// suites, so CI and multi-instance setups can point somewhere else:
//   MM_SITE_URL=http://localhost:8066 npm test
export const baseURL = process.env.MM_SITE_URL || 'http://localhost:8065';

// The system admin these tests log in as.
//
// These defaults are the standard Mattermost development credentials, created by
// `make test-data` in a mattermost-server checkout — they are not specific to any
// one machine. The env var overrides are a convenience for pointing at a server
// with different credentials; they are not a Mattermost-wide convention, so
// nothing outside this repo sets them.
export const adminUsername = process.env.MM_ADMIN_USERNAME || 'sysadmin';
export const adminPassword = process.env.MM_ADMIN_PASSWORD || 'Sys@dmin-sample1';

// Where global-setup.ts saves the admin's authenticated browser state, so specs
// can skip the login screen entirely.
//
// This file contains a live session cookie and must never be committed. If you
// rename it, update `.gitignore` in this directory to match — the two are coupled
// and nothing will warn you if they drift.
export const adminStorageStatePath = 'adminStorageState.json';

// CHANGE ME WHEN YOU FORK THIS TEMPLATE.
//
// Must match the "id" field in plugin.json. Every URL below is derived from it,
// so if the two disagree the tests will 404 with no obvious explanation.
export const pluginID = 'com.mattermost.user-attribute-sync-starter-template';

// Admin console URL for this plugin's settings section. The System Console
// derives this path from the plugin ID.
export const pluginSettingsURL = `${baseURL}/admin_console/plugins/plugin_${pluginID}`;

// Plugin REST routes, registered in server/http_hooks.go.
export const userAttributesURL = `${baseURL}/plugins/${pluginID}/user_attributes`;
export const userAttributesStatusURL = `${userAttributesURL}/status`;

// Values of the AttributeProvider setting, mirroring the constants in
// server/configuration.go.
export const providerFile = 'FileProvider';
export const providerKVStore = 'KVStore';

// Fixture files.
//
// The valid case deliberately reuses the repository's own documented sample file
// rather than a copy, so the tests keep proving that the file shipped in `data/`
// is actually uploadable. The invalid cases live in `assets/` because they have
// no other home.
export const validAttributesFile = '../data/user_attributes.json';
export const notAnArrayFile = './assets/wrong_shape_not_an_array.json';
export const arrayOfArraysFile = './assets/wrong_shape_array_of_arrays.json';
export const malformedFile = './assets/malformed.json';
