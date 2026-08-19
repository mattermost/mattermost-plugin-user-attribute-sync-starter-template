import React from 'react';

import UploadUserAttributes from './upload_user_attributes';

import './attribute_provider.scss';

type Provider = 'FileProvider' | 'KVStore'

type Props = {
    id: string;
    value?: Provider;
    onChange: (id: string, value: Provider) => void;
    disabled?: boolean;

    // This setting is pinned by an environment variable, so the choice cannot be changed here.
    // It applies to the radios only — see the note on the upload panel below.
    setByEnv?: boolean;
};

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
