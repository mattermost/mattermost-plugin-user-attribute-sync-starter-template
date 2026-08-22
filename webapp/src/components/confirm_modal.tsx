import React from 'react';
import {Modal} from 'react-bootstrap';

type Props = {
    show: boolean;
    title: React.ReactNode;
    message: React.ReactNode;
    confirmButtonText?: string;
    cancelButtonText?: string;
    isConfirmDisabled?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
}

/**
 * ConfirmModal guards a destructive action behind an explicit confirmation.
 *
 * The Mattermost webapp has a GenericModal for this, but it is not exported to plugins, so this is
 * a local equivalent built on react-bootstrap — which is a webpack external (see
 * webpack.config.js) supplied by the host webapp at runtime, not an installed dependency. Only its
 * types are in package.json; do not add react-bootstrap itself.
 *
 * The GenericModal__* and ConfirmModal__* class names are the webapp's own, reused so the dialog
 * matches the rest of the System Console rather than looking like a plugin bolted on.
 */
export default function ConfirmModal({
    show,
    title,
    message,
    confirmButtonText = 'Confirm',
    cancelButtonText = 'Cancel',
    isConfirmDisabled = false,
    onConfirm,
    onCancel,
}: Props) {
    return (
        <Modal
            id='userAttrSyncConfirmModal'
            dialogClassName='ConfirmModal a11y_modal'
            aria-labelledby='userAttrSyncConfirmModalLabel'
            aria-modal='true'
            show={show}
            onHide={onCancel}
            restoreFocus={true}
            enforceFocus={true}
        >
            <div className='GenericModal__wrapper'>
                <Modal.Header closeButton={false}>
                    <h1
                        id='userAttrSyncConfirmModalLabel'
                        className='modal-title'
                    >{title}</h1>
                    <button
                        type='button'
                        className='close'
                        aria-label='Close'
                        onClick={onCancel}
                    >
                        <span aria-hidden='true'>{'x'}</span>
                    </button>
                </Modal.Header>
                <Modal.Body>
                    <div className='GenericModal__body padding'>
                        <div className='ConfirmModal__body'>{message}</div>
                        <div className='ConfirmModal__footer'>
                            <button
                                type='button'
                                className='btn btn-tertiary'
                                onClick={onCancel}
                            >{cancelButtonText}</button>
                            <button
                                type='button'
                                className='btn btn-danger'
                                disabled={isConfirmDisabled}
                                autoFocus={true}
                                onClick={onConfirm}
                            >{confirmButtonText}</button>
                        </div>
                    </div>
                </Modal.Body>
            </div>
        </Modal>
    );
}
