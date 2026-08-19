import React from 'react';

import UploadUserAttributes from './upload_user_attributes';

import './attribute_provider.scss';

// Must match the ConfigAttributeProvider* constants in server/configuration.go.
type Provider = 'FileProvider' | 'KVStore'

// The System Console passes more props than this (label, helpText, config, license,
// registerSaveAction, ...); these are the ones this setting needs. See buildCustomSetting in the
// server repo's schema_admin_settings.tsx for the full set.
type Props = {
    id: string;
    value?: Provider;

    // Reports the new value to the admin console, which marks the setting dirty. Nothing is
    // persisted until the admin clicks Save, at which point the server rebuilds the provider.
    onChange: (id: string, value: Provider) => void;

    // The plugin is disabled, or the admin lacks write access to plugin settings.
    disabled?: boolean;

    // This setting is pinned by an environment variable, so the choice cannot be changed here.
    // It applies to the radios only — see the note on the upload panel below.
    setByEnv?: boolean;
};

/**
 * AttributeProvider renders the "User Attribute Source" setting in the System Console.
 *
 * plugin.json declares the setting as "type": "custom" and index.tsx registers this component for
 * it, because choosing a source is more than a value: the Direct Upload option also needs a way to
 * get a file to the server, which the built-in setting controls cannot do.
 *
 * Radios rather than a dropdown, and inline styles matching the console's own RadioSetting
 * classes, so the setting looks like the ones around it.
 */
export default function AttributeProvider({id, value, onChange, disabled, setByEnv}: Props) {
    // Matches the console's own RadioSetting: an env-pinned value is as good as disabled, because
    // saving a different choice here would not change what the server actually uses.
    const isDisabled = disabled || setByEnv;

    return (
        <div className='AttrProvider__container'>
            <label className={`RadioSetting__label${isDisabled ? ' RadioSetting__label--disabled' : ''}`}>
                <input
                    type='radio'
                    className='RadioSetting__input'
                    name={id}
                    value='FileProvider'
                    checked={value === 'FileProvider'}
                    disabled={isDisabled}
                    onChange={() => onChange(id, 'FileProvider')}
                />
                <span className='RadioSetting__text'>{'Local Filesystem'}</span>
            </label>
            <label className={`RadioSetting__label${isDisabled ? ' RadioSetting__label--disabled' : ''}`}>
                <input
                    type='radio'
                    className='RadioSetting__input'
                    name={id}
                    value='KVStore'
                    checked={value === 'KVStore'}
                    disabled={isDisabled}
                    onChange={() => onChange(id, 'KVStore')}
                />
                <span className='RadioSetting__text'>{'Direct Upload'}</span>
            </label>
            <div className='AttrProvider__details'>
                <h5>{'Source Details'}</h5>
                {value === 'FileProvider' &&
                <p>{'Save file on Mattermost Server as data/user_attributes.json'}</p>
                }
                {/* setByEnv is deliberately not passed down. It says the *setting* is pinned, not
                    that the stored data is — the file lives in the KV store, which no environment
                    variable can populate. Pinning the source to KVStore makes uploading the only
                    way to give the plugin data, so that is exactly when these controls are needed
                    most. */}
                {value === 'KVStore' &&
                <UploadUserAttributes
                    id={id}
                    disabled={disabled}
                />}
            </div>
        </div>
    );
}
