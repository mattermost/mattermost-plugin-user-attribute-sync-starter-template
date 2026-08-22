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
};

// A file the admin has chosen but not yet uploaded. recordCount comes from the validation parse,
// so it can be shown as confirmation of what is about to be sent.
type ChosenFile = {
    file: File;
    recordCount: number;
}

type FileStatus = {
    exists: boolean;
    lastUpdated: string | null;
}

// Mirrors maxFileSizeBytes in server/http_hooks.go. Checking here means an oversized file is
// rejected before it is uploaded; the server enforces the same limit regardless.
const MAX_FILE_BYES = 10 * 1024 * 1024;

// Narrows an untrusted response body to FileStatus, so a malformed or truncated answer degrades to
// "no file" instead of putting undefined into state and rendering it.
function parseFileStatus(body: unknown): FileStatus {
    const record = (body ?? {}) as Record<string, unknown>;
    return {
        exists: Boolean(record.exists),
        lastUpdated: typeof record.lastUpdated === 'string' ? record.lastUpdated : null,
    };
}

// The single line describing what is stored. The timestamp arrives as RFC 3339 from the server and
// is rendered in the admin's own locale and time zone, since it is only ever read by a person.
function describeFileStatus(fileStatus: FileStatus | null): string {
    if (fileStatus === null) {
        return 'Unknown';
    }
    if (!fileStatus.exists) {
        return 'No file on server';
    }
    if (fileStatus.lastUpdated === null) {
        return 'File detected - upload time unknown - please reupload';
    }
    return `File detected - uploaded ${new Date(fileStatus.lastUpdated).toLocaleString()}`;
}

/**
 * UploadUserAttributes manages the attributes file that KVStoreProvider syncs from, through the
 * plugin's own HTTP API in server/http_hooks.go.
 *
 * Unlike the provider choice above it, these actions are immediate — they do not go through the
 * admin console's Save button, because they are operations on stored data rather than changes to a
 * setting. That is also why they can report their own success and failure inline.
 */
export default function UploadUserAttributes({id, disabled = false}: Props) {
    const [pendingFile, setPendingFile] = useState<ChosenFile | null>(null);

    // null until the status check comes back, so the UI does not claim there is no file before
    // it has actually asked.
    const [serverFileStatus, setServerFileStatus] = useState<FileStatus | null>(null);

    // A single in-flight action at a time, which drives both the button labels and their
    // disabled state. Prevents, for example, deleting while an upload is still running.
    // Initialized to page_load and cleared when the initial file status check completes.
    const [status, setStatus] = useState<'init' | 'idle' | 'downloading' | 'uploading' | 'deleting'>('init');
    const [error, setError] = useState<string | null>(null);
    const [success, setSuccess] = useState<string | null>(null);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

    // The file input is hidden, so it is triggered and cleared through this ref.
    const inputRef = useRef<HTMLInputElement>(null);

    // Every action also waits for the current one to finish, so a second click cannot race it.
    const serverHasFile = serverFileStatus?.exists ?? false;
    const canUpload = pendingFile !== null && status === 'idle' && !disabled;
    const canDownload = serverHasFile && status === 'idle' && !disabled;
    const canDelete = serverHasFile && status === 'idle' && !disabled;

    // Validates the chosen file locally and holds it as pending. Nothing is sent until the admin
    // presses Upload, so a mistaken selection costs nothing.
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

            // The same shape check the server does: an array of objects, one per user. Contents
            // are not validated here — the server does not either, and unrecognized fields or
            // unmatched emails surface as warnings in the plugin logs during the next sync.
            const parsed: unknown = JSON.parse(text);
            if (!Array.isArray(parsed) || parsed.length === 0 ||
                parsed.some((r) => typeof r !== 'object' || r === null || Array.isArray(r))) {
                throw new Error('File must be a JSON array of objects');
            }

            setPendingFile({file: newFile, recordCount: parsed.length});
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Could not read file');

            // Clear the input so picking the same file again re-fires onChange, which the browser
            // otherwise skips when the selection has not changed.
            if (inputRef.current) {
                inputRef.current.value = '';
            }
        }
    }

    // Sends the pending file to the plugin.
    async function handleUpload() {
        if (pendingFile === null) {
            return;
        }

        setStatus('uploading');
        setError(null);
        setSuccess(null);

        try {
            const response = await fetch(
                Client4.getAbsoluteUrl(`/plugins/${manifest.id}/user_attributes`),
                Client4.getOptions({method: 'POST', body: pendingFile.file}), //getOptions() provides CSRF Token for POST
            );

            const body = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(body.error ?? `Upload failed (${response.status})`);
            }

            setServerFileStatus(parseFileStatus(body));
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
            const response = await fetch(Client4.getAbsoluteUrl(`/plugins/${manifest.id}/user_attributes`));

            if (!response.ok) {
                throw new Error(`Download failed (${response.status})`);
            }

            // Trigger a browser download from the response body. A plain link to the endpoint
            // would work too, but this way a failed request shows an error in the UI instead of
            // navigating away or silently saving an error page.
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

    // Removes the stored file. Only reachable through the confirmation modal, because there is no
    // copy on the server to restore from and already-synced attribute values are left behind.
    async function handleDelete() {
        setShowDeleteConfirm(false);
        setStatus('deleting');
        setError(null);
        setSuccess(null);

        try {
            const response = await fetch(
                Client4.getAbsoluteUrl(`/plugins/${manifest.id}/user_attributes`),
                Client4.getOptions({method: 'DELETE'}),
            );

            if (!response.ok) {
                throw new Error(`Delete failed (${response.status})`);
            }

            setServerFileStatus({exists: false, lastUpdated: null});
            setSuccess('File Deleted');
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Delete failed');
        } finally {
            setStatus('idle');
        }
    }

    async function checkServerFileStatus() {
        // this is only called on mount, with status state initialized to 'init' above.
        // if that changes this should get its own status similar to the other calls.
        try {
            const response = await fetch(Client4.getAbsoluteUrl(`/plugins/${manifest.id}/user_attributes/status`));
            const body = await response.json().catch(() => ({}));
            if (!response.ok) {
                throw new Error(body.error ?? `File status check failed (${response.status})`);
            }
            setServerFileStatus(parseFileStatus(body));
        } catch (err) {
            setError(err instanceof Error ? err.message : 'File status check failed');
        } finally {
            setStatus('idle');
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
                <p className='UserAttrSync__status'>{describeFileStatus(serverFileStatus)}</p>
                <div className='UserAttrSync__row'>
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
