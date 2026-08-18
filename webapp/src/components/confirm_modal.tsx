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
