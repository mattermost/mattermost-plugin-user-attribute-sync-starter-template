import manifest from 'manifest';
import React, {useRef, useState} from 'react';
import type {ChangeEvent} from 'react';

import {Client4} from 'mattermost-redux/client';
import './upload_user_attributes.scss';

type Props = {
    id: string;
    disabled?: boolean;
    setByEnv?: boolean;
};

export default function UploadUserAttributes({id, disabled = false, setByEnv = false}: Props) {
    const [file, setFile] = useState<File | null>(null);
    const [status, setStatus] = useState<'idle' | 'downloading' | 'uploading'>('idle');
    const [error, setError] = useState<string | null>(null);

    const inputRef = useRef<HTMLInputElement>(null);

    const isDisabled = disabled || setByEnv;
    const canUpload = Boolean(file) && status === 'idle' && !isDisabled;
    const canDownload = status === 'idle' && !isDisabled;

    function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
        setFile(e.target.files?.[0] ?? null);
    }

    async function handleUpload() {
        if (!file) {
            return;
        }

        setStatus('uploading');
        setError(null);

        try {
            const response = await fetch(
                `/plugins/${manifest.id}/user_attributes`,
                Client4.getOptions({method: 'POST', body: file}), //getOptions() provides CSRF Token for POST
            );

            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error ?? `Upload failed (${response.status})`);
            }

            setFile(null);
            if (inputRef.current) {
                inputRef.current.value = '';
            }
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Upload failed');
        } finally {
            setStatus('idle');
        }
    }

    async function handleDownload() {
        setStatus('downloading');
        setError(null);

        try {
            const response = await fetch(`/plugins/${manifest.id}/user_attributes`);

            if (!response.ok) {
                throw new Error(`Download failed (${response.status})`);
            }

            const blob = await response.blob();
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'user_attributes.json';
            a.click();
            URL.revokeObjectURL(url);
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Download failed');
        } finally {
            setStatus('idle');
        }
    }
    return (
        <div id={id}>
            <div className='UserAttrSync__row'>
                <button
                    type='button'
                    className='btn btn-tertiary'
                    disabled={isDisabled}
                    onClick={() => inputRef.current?.click()}
                >{'Choose File'}</button>
                <input
                    ref={inputRef}
                    type='file'
                    accept='.json'
                    style={{display: 'none'}}
                    disabled={isDisabled}
                    onChange={handleFileChange}
                />
                <span>{file?.name ?? 'No file chosen'}</span>
                <button
                    type='button'
                    disabled={!canUpload}
                    onClick={handleUpload}
                >{status === 'uploading' ? 'Uploading...' : 'Upload'}</button>
                <button
                    type='button'
                    disabled={!canDownload}
                    onClick={handleDownload}
                >{status === 'downloading' ? 'Downloading...' : 'Download'}</button>
            </div>
            {error && <div className='error-text'>{error}</div>}
        </div>
    );
}
