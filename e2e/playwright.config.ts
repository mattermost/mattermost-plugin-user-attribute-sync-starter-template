/* eslint-disable no-process-env */
import {defineConfig, devices} from '@playwright/test';

import {baseURL} from './constants';

export default defineConfig({
    testDir: './tests',

    globalSetup: require.resolve('./global-setup'),
    globalTeardown: require.resolve('./global-teardown'),

    // Single worker, deliberately: saving an admin console setting is a
    // read-modify-write of the server-wide /api/v4/config document (see
    // apiSetPluginSetting in utils.ts), so parallel workers would discard one
    // another's changes.
    //
    // If your suite outgrows one worker, take a filesystem lock around the tests
    // that mutate config and leave the rest parallel. mattermost-plugin-calls
    // does this — see acquireLock/releaseLock in its e2e/utils.ts:
    // https://github.com/mattermost/mattermost-plugin-calls/tree/master/e2e
    workers: 1,
    fullyParallel: false,

    retries: 0,

    timeout: 60 * 1000,
    expect: {
        timeout: 10 * 1000,
    },

    use: {
        baseURL,
        viewport: {width: 1280, height: 720},
        trace: 'retain-on-failure',
        screenshot: 'only-on-failure',
    },

    projects: [
        {
            name: 'chromium',
            use: {...devices['Desktop Chrome']},
        },
    ],

    reporter: [
        ['list'],
        ['html', {open: 'never'}],
    ],

    // When you wire these tests into CI, uncomment:
    //
    //   forbidOnly: Boolean(process.env.CI),
    //   retries: process.env.CI ? 1 : 0,
    //   reporter: process.env.CI ? [['list'], ['junit', {outputFile: 'results.xml'}]] : [['list'], ['html', {open: 'never'}]],
});
