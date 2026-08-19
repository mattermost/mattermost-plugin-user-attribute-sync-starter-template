import manifest from 'manifest';
import React, {useRef, useState, useEffect} from 'react';
import type {ChangeEvent} from 'react';

import {Client4} from 'mattermost-redux/client';

import './upload_user_attributes.scss';
import ConfirmModal from './confirm_modal';

type Props = {
    id: string;

    // The plugin is disabled, or the admin lacks write access to plugin settings. Note there is
    // deliberately no setByEnv here: an environment variable can pin which source the plugin uses,
    // but it cannot supply the stored file, so it is no reason to block managing that file.
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
    const [serverHasFile, setServerHasFile] = useState<boolean | null>(null);
    const [status, setStatus] = useState<'idle' | 'downloading' | 'uploading' | 'deleting'>('idle');
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

    const inputRef = useRef<HTMLInputElement>(null);

    // Every action also waits for the current one to finish, so a second click cannot race it.
    const canUpload = pendingFile !== null && status === 'idle' && !disabled;
    const canDownload = serverHasFile && status === 'idle' && !disabled;
    const canDelete = serverHasFile && status === 'idle' && !disabled;

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
        setSuccess(null);

        try {
            const response = await fetch(
                `/plugins/${manifest.id}/user_attributes`,
                Client4.getOptions({method: 'POST', body: pendingFile.file}), //getOptions() provides CSRF Token for POST
            );

            if (!response.ok) {
                const body = await response.json().catch(() => ({}));
                throw new Error(body.error ?? `Upload failed (${response.status})`);
            }

            setServerHasFile(true);
            setPendingFile(null);
            if (inputRef.current) {
                inputRef.current.value = '';
            }
            setSuccess('File uploaded');
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Upload failed');
        } finally {
            setStatus('idle');
        }
    }

    async function handleDownload() {
        setStatus('downloading');
        setSuccess(null);
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

    async function handleDelete() {
        setShowDeleteConfirm(false);
        setStatus('deleting');
        setError(null);
        setSuccess(null);

        try {
            const response = await fetch(
                `/plugins/${manifest.id}/user_attributes`,
                Client4.getOptions({method: 'DELETE'}),
            );

            if (!response.ok) {
                throw new Error(`Delete failed (${response.status})`);
            }

            setServerHasFile(false);
            setSuccess('File Deleted');
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Delete failed');
        } finally {
            setStatus('idle');
        }
    }

    async function checkServerFileStatus() {
        const response = await fetch(`/plugins/${manifest.id}/user_attributes/exists`);
        if (response.ok) {
            const body = await response.json();
            setServerHasFile(Boolean(body.exists));
        }
    }

    useEffect(() => {
        checkServerFileStatus();
    }, []);

    return (
        <div
            id={id}
            className='UserAttrSync'
        >
            <section className='UserAttrSync__section'>
                <h5 className='UserAttrSync__heading'>{'Current file status'}</h5>
                <div className='UserAttrSync__row'>
                    <span>{serverHasFile ? 'File detected' : 'No file on server'}</span>
                    <button
                        type='button'
                        className='btn btn-tertiary'
                        disabled={!canDownload}
                        onClick={handleDownload}
                    >{status === 'downloading' ? 'Downloading...' : 'Download'}</button>
                    <button
                        type='button'
                        className='btn btn-tertiary btn-danger'
                        disabled={!canDelete}
                        onClick={() => setShowDeleteConfirm(true)}
                    >{status === 'deleting' ? 'Deleting...' : 'Delete'}</button>
                </div>
            </section>

            <section className='UserAttrSync__section'>
                <h5 className='UserAttrSync__heading'>{'Upload new file'}</h5>
                <div className='UserAttrSync__row'>
                    <button
                        type='button'
                        className='btn btn-tertiary'
                        disabled={disabled}
                        onClick={() => inputRef.current?.click()}
                    >{'Choose File'}</button>
                    <input
                        ref={inputRef}
                        data-testid='userAttrSyncFileInput'
                        type='file'
                        accept='.json'
                        style={{display: 'none'}}
                        disabled={disabled}
                        onChange={handleFileChange}
                    />
                    <button
                        type='button'
                        className='btn btn-tertiary'
                        disabled={!canUpload}
                        onClick={handleUpload}
                    >{status === 'uploading' ? 'Uploading...' : 'Upload'}</button>
                    {pendingFile && (
                        <span>
                            {`${pendingFile.file.name} - ${pendingFile.recordCount} users - ${Math.round(pendingFile.file.size / 1024)} KB`}
                        </span>
                    )}
                </div>
            </section>

            {error && <div className='error-text'>{error}</div>}
            {success && <div className='success-text'>{success}</div>}

            <ConfirmModal
                show={showDeleteConfirm}
                title={'Delete stored user attributes file?'}
                message={'The uploaded file will be removed from the server and the plugin will have nothing to sync. Attribute values already written to user profiles will remain. This cannot be undone.'}
                confirmButtonText={'Delete'}
                isConfirmDisabled={disabled}
                onConfirm={handleDelete}
                onCancel={() => setShowDeleteConfirm(false)}
            />
        </div>
    );
}
