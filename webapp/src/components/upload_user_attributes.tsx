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

type UserFile = {
    file: File;
    recordCount: number;
}

const MAX_FILE_BYES = 10 * 1024 * 1024;

export default function UploadUserAttributes({id, disabled = false, setByEnv = false}: Props) {
    const [pendingFile, setPendingFile] = useState<UserFile | null>(null);
    const [status, setStatus] = useState<'idle' | 'downloading' | 'uploading'>('idle');
    const [error, setError] = useState<string | null>(null);

    const inputRef = useRef<HTMLInputElement>(null);

    const isDisabled = disabled || setByEnv;
    const canUpload = pendingFile !== null && status === 'idle' && !isDisabled;
    const canDownload = status === 'idle' && !isDisabled;

    async function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
        const newFile = e.target.files?.[0] ?? null;
        setError(null);
        setPendingFile(null);

        if (!newFile) {
            return;
        }

        try {
            if (newFile.size > MAX_FILE_BYES) {
                throw new Error('File exceeds the 10 MB limit');
            }
            const text = await newFile.text();

            const parsed: unknown = JSON.parse(text);
            if (!Array.isArray(parsed) ||
                parsed.some((r) => typeof r !== 'object' || r === null || Array.isArray(r))) {
                throw new Error('File must be a JSON array of objects');
            }

            setPendingFile({file: newFile, recordCount: parsed.length});
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Could not read file');
            if (inputRef.current) {
                inputRef.current.value = '';
            }
        }
    }

    async function handleUpload() {
        if (pendingFile === null) {
            return;
        }

        setStatus('uploading');
        setError(null);

        try {
            const response = await fetch(
                `/plugins/${manifest.id}/user_attributes`,
                Client4.getOptions({method: 'POST', body: pendingFile.file}), //getOptions() provides CSRF Token for POST
            );

            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error ?? `Upload failed (${response.status})`);
            }

            setPendingFile(null);
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
                {pendingFile && (
                    <span>
                        {`${pendingFile.file.name} - ${pendingFile.recordCount} users - ${Math.round(pendingFile.file.size / 1024)} KB`}
                    </span>
                )}
                <button
                    type='button'
                    className='btn btn-tertiary'
                    disabled={!canUpload}
                    onClick={handleUpload}
                >{status === 'uploading' ? 'Uploading...' : 'Upload'}</button>
                <button
                    type='button'
                    className='btn btn-tertiary'
                    disabled={!canDownload}
                    onClick={handleDownload}
                >{status === 'downloading' ? 'Downloading...' : 'Download'}</button>
            </div>
            {error && <div className='error-text'>{error}</div>}
        </div>
    );
}
