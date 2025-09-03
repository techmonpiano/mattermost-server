// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {
    FloatingFocusManager,
    autoUpdate,
    flip,
    offset,
    safePolygon,
    shift,
    useFloating,
    useHover,
    useId,
    useInteractions,
    useRole,
} from '@floating-ui/react';
import classNames from 'classnames';
import React, {memo, useState} from 'react';
import {FormattedMessage} from 'react-intl';
import {useDispatch} from 'react-redux';

import {CheckCircleIcon} from '@mattermost/compass-icons/components';
import type {Post} from '@mattermost/types/posts';
import type {UserProfile} from '@mattermost/types/users';

import {markPostAsRead, unmarkPostAsRead} from 'mattermost-redux/actions/posts';

import PostReadReceiptsUserPopover from './post_read_receipts_users_popover';

import './post_read_receipts.scss';

type Props = {
    authorId: UserProfile['id'];
    currentUserId: UserProfile['id'];
    hasReactions: boolean;
    isDeleted: boolean;
    list?: Array<{user: UserProfile; readAt: number}>;
    postId: Post['id'];
    showDivider?: boolean;
}

function PostReadReceipts({
    authorId,
    currentUserId,
    hasReactions,
    isDeleted,
    list,
    postId,
    showDivider = true,
}: Props) {
    let readAt = 0;
    const headingId = useId();
    const isCurrentAuthor = authorId === currentUserId;
    const dispatch = useDispatch();
    const [open, setOpen] = useState(false);

    if (list && list.length) {
        const receipt = list.find((receipt) => receipt.user.id === currentUserId);
        if (receipt) {
            readAt = receipt.readAt;
        }
    }
    
    // Authors cannot mark their own posts as read
    const buttonDisabled = isCurrentAuthor;

    const {x, y, strategy, context, refs: {setReference, setFloating}} = useFloating({
        open,
        onOpenChange: setOpen,
        placement: 'top-start',
        whileElementsMounted: autoUpdate,
        middleware: [
            offset(5),
            flip({
                fallbackPlacements: ['bottom-start', 'right'],
                padding: 12,
            }),
            shift({
                padding: 12,
            }),
        ],
    });

    const {getReferenceProps, getFloatingProps} = useInteractions([
        useHover(context, {
            enabled: list && list.length > 0,
            mouseOnly: true,
            delay: {
                open: 300,
                close: 0,
            },
            restMs: 100,
            handleClose: safePolygon({
                blockPointerEvents: false,
            }),
        }),
        useRole(context),
    ]);

    const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
        if (buttonDisabled) {
            e.preventDefault();
            e.stopPropagation();
            return;
        }
        if (readAt) {
            dispatch(unmarkPostAsRead(postId));
        } else {
            dispatch(markPostAsRead(postId));
        }
    };

    if (isDeleted) {
        return null;
    }

    let buttonText: React.ReactNode = (
        <FormattedMessage
            id={'post_read_receipts.button.mark_read'}
            defaultMessage={'Mark as read'}
        />
    );

    if ((list && list.length) || isCurrentAuthor) {
        buttonText = list?.length || 0;
    }

    const button = (
        <>
            <button
                ref={setReference}
                onClick={handleClick}
                className={classNames({
                    ReadReceiptButton: true,
                    'ReadReceiptButton--read': Boolean(readAt),
                    'ReadReceiptButton--disabled': buttonDisabled,
                    'ReadReceiptButton--default': !list || list.length === 0,
                })}
                {...getReferenceProps()}
            >
                <CheckCircleIcon size={16}/>
                {buttonText}
            </button>
            {showDivider && hasReactions && <div className='ReadReceiptButton__divider'/>}
        </>
    );

    if (!list || !list.length) {
        return button;
    }

    return (
        <>
            {button}
            {open && (
                <FloatingFocusManager
                    context={context}
                    modal={false}
                >
                    <div
                        ref={setFloating}
                        style={{
                            position: strategy,
                            top: y ?? 0,
                            left: x ?? 0,
                            width: 248,
                            zIndex: 999,
                        }}
                        aria-labelledby={headingId}
                        {...getFloatingProps()}
                    >
                        <PostReadReceiptsUserPopover
                            currentUserId={currentUserId}
                            list={list}
                        />
                    </div>
                </FloatingFocusManager>
            )}
        </>
    );
}

export default memo(PostReadReceipts);