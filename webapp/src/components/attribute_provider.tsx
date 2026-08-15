import React from 'react';

import UploadUserAttributes from './upload_user_attributes';

import './attribute_provider.scss';

type Provider = 'FileProvider' | 'KVStore'

type Props = {
    id: string;
    value?: Provider;
    onChange: (id: string, value: Provider) => void;
    disabled?: boolean;
    setByEnv?: boolean;
};

export default function AttributeProvider({id, value, onChange, disabled, setByEnv}: Props) {
    return (
        <div className='AttrProvider__container'>
            <label className={`RadioSetting__label${disabled ? ' RadioSetting__label--disabled' : ''}`}>
                <input
                    type='radio'
                    className='RadioSetting__input'
                    name={id}
                    value='FileProvider'
                    checked={value === 'FileProvider'}
                    disabled={disabled}
                    onChange={() => onChange(id, 'FileProvider')}
                />
                <span className='RadioSetting__text'>{'Local Filesystem'}</span>
            </label>
            <label className={`RadioSetting__label${disabled ? ' RadioSetting__label--disabled' : ''}`}>
                <input
                    type='radio'
                    className='RadioSetting__input'
                    name={id}
                    value='KVStore'
                    checked={value === 'KVStore'}
                    disabled={disabled}
                    onChange={() => onChange(id, 'KVStore')}
                />
                <span className='RadioSetting__text'>{'Direct Upload'}</span>
            </label>
            {value === 'FileProvider' &&
            <p>{'Save file on Mattermost Server as data/user_attributes.json'}</p>
            }
            {value === 'KVStore' &&
            <UploadUserAttributes
                id={id}
                disabled={disabled}
                setByEnv={setByEnv}
            />}
        </div>
    );
}
