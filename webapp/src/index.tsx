// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import manifest from 'manifest';
import type {Store, Action} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import AttributeProvider from 'components/attribute_provider';

import type {PluginRegistry} from 'types/mattermost-webapp';

export default class Plugin {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-function
    public async initialize(registry: PluginRegistry, store: Store<GlobalState, Action<Record<string, unknown>>>) {
        // @see https://developers.mattermost.com/extend/plugins/webapp/reference/

        // This plugin's webapp bundle exists solely to render one System Console setting. The
        // first argument must match the setting key in plugin.json, which declares it as
        // "type": "custom" so the console defers to this component. showTitle lets the console
        // draw the setting's label next to it, so it lines up with the settings above.
        registry.registerAdminConsoleCustomSetting('AttributeProvider', AttributeProvider, {showTitle: true});
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
