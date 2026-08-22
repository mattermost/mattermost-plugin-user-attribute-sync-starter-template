import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import AttributeProvider from './attribute_provider';

let fetchMock: jest.Mock;

beforeEach(() => {
    // The upload panel asks the server what is stored as soon as it mounts, and jsdom provides no
    // fetch. Answering "no file" keeps that out of the way of these tests.
    fetchMock = jest.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({exists: false, lastUpdated: null}),
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;
});

// Renders the setting and lets the upload panel's mount-time request settle, so the state update it
// triggers happens inside act() rather than after the test has finished.
const renderSetting = async (props: Partial<React.ComponentProps<typeof AttributeProvider>> = {}) => {
    const result = render(
        <AttributeProvider
            id='AttributeProvider'
            value='KVStore'
            onChange={jest.fn()}
            {...props}
        />,
    );

    await act(async () => {
        await Promise.resolve();
    });

    return result;
};

test('shows the upload controls for the KVStore source', async () => {
    await renderSetting({value: 'KVStore'});

    expect(screen.getByRole('button', {name: 'Choose File'})).toBeInTheDocument();
});

test('hides the upload controls for the FileProvider source', async () => {
    await renderSetting({value: 'FileProvider'});

    // queryBy rather than getBy: getBy throws when it finds nothing, so it cannot express absence.
    expect(screen.queryByRole('button', {name: 'Choose File'})).not.toBeInTheDocument();
});

test('reports the selected source when a radio is chosen', async () => {
    const onChange = jest.fn();
    await renderSetting({value: 'KVStore', onChange});

    await userEvent.click(screen.getByRole('radio', {name: 'Local Filesystem'}));

    expect(onChange).toHaveBeenCalledWith('AttributeProvider', 'FileProvider');
});

test('disables the source radios when the setting is set by environment variable', async () => {
    await renderSetting({setByEnv: true});

    expect(screen.getByRole('radio', {name: 'Local Filesystem'})).toBeDisabled();
    expect(screen.getByRole('radio', {name: 'Direct Upload'})).toBeDisabled();
});

test('keeps the upload controls usable when the setting is set by environment variable', async () => {
    await renderSetting({setByEnv: true});

    expect(screen.getByRole('button', {name: 'Choose File'})).toBeEnabled();
});

test('disables the upload controls when the plugin itself is disabled', async () => {
    await renderSetting({disabled: true});

    expect(screen.getByRole('button', {name: 'Choose File'})).toBeDisabled();
});
