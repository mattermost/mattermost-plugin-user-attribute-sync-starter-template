import {expect, Page} from '@playwright/test';

import {pluginID, pluginSettingsURL} from '../constants';

// The visible labels of the AttributeProvider radios. Renaming a label in
// attribute_provider.tsx means changing it here too; every call site is then a
// compile error until updated.
export type ProviderLabel = 'Local Filesystem' | 'Direct Upload';

/**
 * Page object for this plugin's System Console settings section.
 *
 * If your plugin adds another surface — a channel header menu, a right-hand
 * sidebar — add a sibling file here rather than extending this one.
 */
export default class PluginSettingsPage {
    readonly page: Page;

    constructor(page: Page) {
        this.page = page;
    }

    async goto() {
        await this.page.goto(pluginSettingsURL);
        await expect(this.page.getByTestId('plugin-metadata-id')).toHaveText(pluginID);
    }

    // --- locators ----------------------------------------------------------

    providerRadio(label: ProviderLabel) {
        return this.page.getByRole('radio', {name: label});
    }

    chooseFileButton() {
        return this.page.getByRole('button', {name: 'Choose File'});
    }

    uploadButton() {
        return this.page.getByRole('button', {name: 'Upload'});
    }

    downloadButton() {
        return this.page.getByRole('button', {name: 'Download'});
    }

    deleteButton() {
        return this.page.locator('.UserAttrSync').getByRole('button', {name: 'Delete'});
    }

    // Need to use a CSS id here since getByRole will
    confirmDeleteDialog() {
        return this.page.getByRole('dialog', {name: 'Delete stored user attributes file?'});
    }

    confirmDeleteButton() {
        return this.confirmDeleteDialog().getByRole('button', {name: 'Delete'});
    }

    cancelDeleteButton() {
        return this.confirmDeleteDialog().getByRole('button', {name: 'Cancel'});
    }

    saveButton() {
        return this.page.getByRole('button', {name: 'Save'});
    }

    // The file input is display:none, so it has no accessible role to locate it
    // by. That is why it carries a data-testid where nothing else here does.
    fileInput() {
        return this.page.getByTestId('userAttrSyncFileInput');
    }

    /** The "N users - N KB" line shown after picking a valid file. */
    pendingSummary() {
        return this.page.getByText(/users/);
    }

    errorText() {
        return this.page.locator('.error-text');
    }

    successText() {
        return this.page.locator('.success-text');
    }

    // --- actions -----------------------------------------------------------

    async selectProvider(label: ProviderLabel) {
        await this.providerRadio(label).check();
    }

    async save() {
        await this.saveButton().click();
    }

    async chooseFile(filePath: string) {
        await this.fileInput().setInputFiles(filePath);
    }

    async chooseAndUpload(filePath: string) {
        await this.chooseFile(filePath);
        await this.uploadButton().click();
    }

    async deleteWithConfirm() {
        await this.deleteButton().click();
        await this.confirmDeleteButton().click();
    }

    // --- assertions --------------------------------------------------------

    async expectFileOnServer() {
        await expect(this.page.getByText('File detected')).toBeVisible();
    }

    async expectNoFileOnServer() {
        await expect(this.page.getByText('No file on server')).toBeVisible();
    }

    async expectUploadControlsVisible(visible: boolean) {
        if (visible) {
            await expect(this.chooseFileButton()).toBeVisible();
        } else {
            await expect(this.chooseFileButton()).toBeHidden();
        }
    }
}
