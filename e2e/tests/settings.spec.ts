import {expect, test} from '@playwright/test';

import {adminStorageStatePath, providerKVStore} from '../constants';
import PluginSettingsPage from '../pages/plugin_settings_page';
import {adminAPIContext, apiSetPluginSetting} from '../utils';

test.describe('user attribute sync settings', () => {
    test.use({storageState: adminStorageStatePath});

    test.beforeEach(async () => {
        const admin = await adminAPIContext();

        // Mattermost lowercases plugin setting keys when it stores them, so this
        // is "attributeprovider" even though plugin.json declares
        // "AttributeProvider".
        await apiSetPluginSetting(admin, 'attributeprovider', providerKVStore);

        await admin.dispose();
    });

    test('renders both attribute provider options', async ({page}) => {
        const settings = new PluginSettingsPage(page);
        await settings.goto();

        await expect(settings.providerRadio('Local Filesystem')).toBeVisible();
        await expect(settings.providerRadio('Direct Upload')).toBeVisible();
        await expect(settings.providerRadio('Direct Upload')).toBeChecked();
    });

    test('shows the upload controls only for the Direct Upload provider', async ({page}) => {
        const settings = new PluginSettingsPage(page);
        await settings.goto();

        await settings.selectProvider('Local Filesystem');
        await settings.expectUploadControlsVisible(false);

        await settings.selectProvider('Direct Upload');
        await settings.expectUploadControlsVisible(true);
    });
});
