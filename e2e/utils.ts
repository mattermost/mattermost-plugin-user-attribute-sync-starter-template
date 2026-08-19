import {APIRequestContext, request} from '@playwright/test';
import * as fs from 'fs/promises';

import {
    adminPassword,
    adminStorageStatePath,
    adminUsername,
    baseURL,
    pluginID,
    userAttributesURL,
} from './constants';

/**
 * Headers every authenticated Mattermost API call needs.
 *
 * Mattermost rejects cookie-authenticated non-GET requests without a CSRF token,
 * issued as the MMCSRF cookie at login.
 */
export async function getHTTPHeaders(context: APIRequestContext) {
    const state = await context.storageState();
    const csrf = state.cookies.find((cookie) => cookie.name === 'MMCSRF')?.value ?? '';

    return {
        'X-CSRF-Token': csrf,
        'X-Requested-With': 'XMLHttpRequest',
    };
}

/**
 * Logs in as the system admin and returns the authenticated context.
 *
 * Called once from global-setup.ts, which saves the resulting storage state so specs can start
 * already logged in rather than driving the login form each time.
 */
export async function loginAsAdmin() {
    const context = await request.newContext({
        baseURL,

        // Suppresses the "open this in the desktop app?" interstitial, which would
        // otherwise cover the page on first load.
        storageState: {
            cookies: [],
            origins: [{
                origin: baseURL,
                localStorage: [{name: '__landingPageSeen__', value: 'true'}],
            }],
        },
    });

    const response = await context.post('/api/v4/users/login', {
        data: {login_id: adminUsername, password: adminPassword},
        headers: {'X-Requested-With': 'XMLHttpRequest'},
    });

    if (!response.ok()) {
        throw new Error(
            `Could not log in as "${adminUsername}" (HTTP ${response.status()}). ` +
            'Is the server running, and does that system admin exist? ' +
            'See e2e/README.md for prerequisites.',
        );
    }

    return context;
}

/**
 * An API context reusing the session global-setup.ts saved, for tests that need to set up or clean
 * up through the API. Dispose of it when done, or Playwright reports an open handle.
 */
export async function adminAPIContext() {
    return request.newContext({baseURL, storageState: adminStorageStatePath});
}

/** Enables the plugin, so its settings section and HTTP routes exist. */
export async function apiEnablePlugin(context: APIRequestContext) {
    const headers = await getHTTPHeaders(context);
    const response = await context.post(`/api/v4/plugins/${pluginID}/enable`, {headers});

    // A 404 here means the plugin is not installed on the server. Failing in
    // setup gives a usable message; without this the first test instead times out
    // waiting for a settings page that was never going to render.
    if (!response.ok()) {
        throw new Error(
            `Could not enable plugin "${pluginID}" (HTTP ${response.status()}). ` +
            'Run `make deploy` from the repo root to install it on the server, ' +
            'and check that pluginID in constants.ts matches plugin.json.',
        );
    }
}

/**
 * Sets one of this plugin's settings.
 *
 * Plugin settings live in a nested map at PluginSettings.Plugins[pluginID], and
 * /api/v4/config is a whole-document PUT — there is no endpoint for patching a
 * single key. This read-modify-write of shared server state is why
 * playwright.config.ts runs a single worker.
 */
export async function apiSetPluginSetting(context: APIRequestContext, key: string, value: unknown) {
    const headers = await getHTTPHeaders(context);
    const config = await (await context.get('/api/v4/config', {headers})).json();

    config.PluginSettings.Plugins = {
        ...config.PluginSettings.Plugins,
        [pluginID]: {
            ...config.PluginSettings.Plugins[pluginID],
            [key]: value,
        },
    };

    await context.put('/api/v4/config', {data: config, headers});
}

/**
 * Dismisses the tutorial and onboarding flows for the logged-in user.
 *
 * The onboarding checklist renders a transparent full-viewport overlay in
 * #root-portal that silently swallows every click, including in the System
 * Console. These are the same preferences Mattermost's own Playwright suite sets
 * for every user it creates — see lib/src/server/user.ts in
 * mattermost/e2e-tests/playwright.
 */
export async function apiDismissOnboarding(context: APIRequestContext) {
    const headers = await getHTTPHeaders(context);
    const me = await (await context.get('/api/v4/users/me', {headers})).json();

    await context.put(`/api/v4/users/${me.id}/preferences`, {
        headers,
        data: [
            {user_id: me.id, category: 'tutorial_step', name: me.id, value: '999'},
            {user_id: me.id, category: 'crt_thread_pane_step', name: me.id, value: '999'},
            {user_id: me.id, category: 'onboarding_task_list', name: 'onboarding_task_list_show', value: 'false'},
            {user_id: me.id, category: 'onboarding_task_list', name: 'onboarding_task_list_open', value: 'false'},
        ],
    });
}

/** Removes any stored attributes file, so a spec starts from a known state. */
export async function apiDeleteStoredAttributes(context: APIRequestContext) {
    const headers = await getHTTPHeaders(context);
    await context.delete(userAttributesURL, {headers});
}

/**
 * Stores an attributes file directly, for tests that need one already present
 * without exercising the upload UI to get there.
 */
export async function apiUploadStoredAttributes(context: APIRequestContext, filePath: string) {
    const headers = await getHTTPHeaders(context);
    const response = await context.post(userAttributesURL, {
        headers,
        data: await fs.readFile(filePath),
    });

    if (!response.ok()) {
        throw new Error(`Could not seed attributes from ${filePath} (HTTP ${response.status()})`);
    }
}
