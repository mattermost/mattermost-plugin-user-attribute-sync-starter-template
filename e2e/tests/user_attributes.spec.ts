import {expect, test} from '@playwright/test';
import * as fs from 'fs/promises';

import {
    adminStorageStatePath,
    arrayOfArraysFile,
    malformedFile,
    notAnArrayFile,
    providerKVStore,
    validAttributesFile,
} from '../constants';
import PluginSettingsPage from '../pages/plugin_settings_page';
import {
    adminAPIContext,
    apiDeleteStoredAttributes,
    apiSetPluginSetting,
    apiUploadStoredAttributes,
} from '../utils';

// data/user_attributes.json ships with three records.
const validRecordCount = 3;

test.describe('user attributes upload and download', () => {
    test.use({storageState: adminStorageStatePath});

    test.beforeEach(async () => {
        const admin = await adminAPIContext();
        await apiSetPluginSetting(admin, 'attributeprovider', providerKVStore);
        await apiDeleteStoredAttributes(admin);
        await admin.dispose();
    });

    test('uploads a valid attributes file', async ({page}) => {
        const settings = new PluginSettingsPage(page);
        await settings.goto();

        await settings.expectNoFileOnServer();

        await settings.chooseFile(validAttributesFile);
        await expect(settings.pendingSummary()).toContainText(`${validRecordCount} users`);

        await settings.uploadButton().click();

        await settings.expectFileOnServer();
        await expect(settings.errorText()).toHaveCount(0);
        await expect(settings.successText()).toBeVisible();
    });

    const invalidFiles = [
        {name: 'a JSON object rather than an array', path: notAnArrayFile},
        {name: 'an array of arrays', path: arrayOfArraysFile},
        {name: 'text that is not JSON', path: malformedFile},
    ];

    for (const invalid of invalidFiles) {
        test(`rejects ${invalid.name}`, async ({page}) => {
            const settings = new PluginSettingsPage(page);
            await settings.goto();

            await settings.chooseFile(invalid.path);

            await expect(settings.errorText()).toBeVisible();
            await expect(settings.successText()).toHaveCount(0);
            await expect(settings.uploadButton()).toBeDisabled();
            await settings.expectNoFileOnServer();
        });
    }

    test('downloads the stored file', async ({page}) => {
        const admin = await adminAPIContext();
        await apiUploadStoredAttributes(admin, validAttributesFile);
        await admin.dispose();

        const settings = new PluginSettingsPage(page);
        await settings.goto();
        await settings.expectFileOnServer();

        // Start listening before the click: the download can fire before an await
        // placed after it would have subscribed.
        const downloadPromise = page.waitForEvent('download');
        await settings.downloadButton().click();
        const download = await downloadPromise;

        expect(download.suggestedFilename()).toBe('user_attributes.json');

        const downloadedPath = await download.path();
        const contents: unknown = JSON.parse(await fs.readFile(downloadedPath, 'utf8'));
        const original: unknown = JSON.parse(await fs.readFile(validAttributesFile, 'utf8'));

        expect(contents).toEqual(original);
    });

    test('deletes the stored file', async ({page}) => {
        const admin = await adminAPIContext();
        await apiUploadStoredAttributes(admin, validAttributesFile);
        await admin.dispose();

        const settings = new PluginSettingsPage(page);
        await settings.goto();
        await settings.expectFileOnServer();

        await settings.deleteWithConfirm();

        await settings.expectNoFileOnServer();
        await expect(settings.downloadButton()).toBeDisabled();
        await expect(settings.successText()).toBeVisible();
    });

     test('keeps the stored file when the delete modal is dismissed', async ({page}) => {
        const admin = await adminAPIContext();
        await apiUploadStoredAttributes(admin, validAttributesFile);
        await admin.dispose();

        const settings = new PluginSettingsPage(page);
        await settings.goto();
        await settings.expectFileOnServer();

        await settings.deleteButton().click();
        await settings.cancelDeleteButton().click();

        await expect(settings.confirmDeleteDialog()).toBeHidden()
        await settings.expectFileOnServer();
        await expect(settings.downloadButton()).toBeEnabled();
    });
});
