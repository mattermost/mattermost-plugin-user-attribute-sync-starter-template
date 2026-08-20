// Stands in for react-bootstrap in tests.
//
// react-bootstrap is a webpack external (see webpack.config.js): the Mattermost webapp provides it
// at runtime, so it is deliberately not installed here and jest cannot resolve it. Any component
// that renders a Modal would fail to load without this. Mapped in place of the real package by
// moduleNameMapper in package.json.
//
// Only the parts this plugin uses are implemented. If you use more of react-bootstrap, extend this.
//
// Installing the real package as a devDependency would not affect the bundle — `externals` excludes
// it by configuration, not by absence — but it is still the wrong move here, for two reasons:
//
//  1. The Mattermost webapp does not use upstream react-bootstrap. It uses its own long-lived fork
//     (github:mattermost/react-bootstrap), so an upstream install would test against a different
//     library than the one that runs in production.
//  2. Upstream 0.32.x, the line matching the fork and the pinned @types, declares its react peer as
//     ">=15.3.0". npm satisfies that upper bound with react-dom 19, which collides with react 17;
//     installing needs either an `overrides` pin or --legacy-peer-deps, and the latter drops
//     react-dom from the tree entirely and breaks React Testing Library.
//
// The real Modal is exercised where it is actually meaningful: the Playwright suite in e2e/ drives
// the delete-confirmation dialog in a real browser against the host's real fork.

import React from 'react';

type ModalProps = {
    show?: boolean;
    children?: React.ReactNode;
};

// Matches the real Modal in the one way tests depend on: it renders nothing while show is false.
export const Modal = ({show, children}: ModalProps) => (show ? <div role='dialog'>{children}</div> : null);

Modal.Header = ({children}: {children?: React.ReactNode}) => <div>{children}</div>;
Modal.Body = ({children}: {children?: React.ReactNode}) => <div>{children}</div>;
Modal.Footer = ({children}: {children?: React.ReactNode}) => <div>{children}</div>;
Modal.Title = ({children}: {children?: React.ReactNode}) => <div>{children}</div>;
