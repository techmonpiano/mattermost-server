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

import {CheckCircleIcon} from '@mattermost/compass-icons/components';
import type {Post} from '@mattermost/types/posts';
import type {UserProfile} from '@mattermost/types/users';


import PostReadReceiptsUserPopover from './post_read_receipts_users_popover';

import './post_read_receipts.scss';

export type ReadReceiptData = {
    user: UserProfile;
    readAt: number;
};

type Props = {
    postId: Post['id'];
    currentUserId: UserProfile['id'];
    authorId: UserProfile['id'];
    isDeleted: boolean;
    hasReactions: boolean;
    readReceipts?: ReadReceiptData[];
    showDivider?: boolean;
    channelType: string;
    isReadReceiptsEnabled: boolean;
}

function PostReadReceipts({
    postId,
    currentUserId,
    authorId,
    isDeleted,
    hasReactions,
    readReceipts = [],
    showDivider = true,
    channelType,
    isReadReceiptsEnabled,
}: Props) {
    const headingId = useId();
    const [open, setOpen] = useState(false);
    
    // Debug logging - temporarily show component info
    console.log('PostReadReceipts Debug:', {
        postId,
        channelType,
        isReadReceiptsEnabled,
        isDeleted,
        readReceiptsCount: readReceipts?.length || 0,
        hasReactions,
    });
    
    // Don't show if read receipts are disabled
    if (!isReadReceiptsEnabled) {
        return null;
    }
    
    // Don't show if post is deleted
    if (isDeleted) {
        return null;
    }
    
    // Don't show if no read receipts
    if (!readReceipts || readReceipts.length === 0) {
        return null;
    }
    
    // Filter out current user and author from display
    const filteredReceipts = readReceipts.filter(
        receipt => receipt.user.id !== currentUserId && receipt.user.id !== authorId
    );
    
    if (filteredReceipts.length === 0) {
        return null;
    }

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
            enabled: filteredReceipts.length > 0,
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
    
    // Get the most recent read time for display
    const mostRecentRead = Math.max(...filteredReceipts.map(r => r.readAt));
    
    const renderIndicator = () => {
        if (filteredReceipts.length === 1) {
            // Single user - show their avatar
            const receipt = filteredReceipts[0];
            return (
                <div className='ReadReceiptIndicator ReadReceiptIndicator--single'>
                    <img
                        className='ReadReceiptIndicator__avatar'
                        src={`/api/v4/users/${receipt.user.id}/image?_=${receipt.user.last_picture_update}`}
                        alt={receipt.user.username}
                    />
                    <CheckCircleIcon 
                        size={12} 
                        className='ReadReceiptIndicator__check'
                    />
                </div>
            );
        } else {
            // Multiple users - show count with check icon
            return (
                <div className='ReadReceiptIndicator ReadReceiptIndicator--multiple'>
                    <CheckCircleIcon 
                        size={16} 
                        className='ReadReceiptIndicator__check'
                    />
                    <span className='ReadReceiptIndicator__count'>
                        {filteredReceipts.length}
                    </span>
                </div>
            );
        }
    };

    const indicator = (
        <>
            <div
                ref={setReference}
                className={classNames('ReadReceiptIndicator__container', {
                    'ReadReceiptIndicator__container--with-divider': showDivider && hasReactions,
                })}
                {...getReferenceProps()}
            >
                {renderIndicator()}
            </div>
            {showDivider && hasReactions && <div className='ReadReceiptIndicator__divider'/>}
        </>
    );

    if (filteredReceipts.length === 0) {
        return indicator;
    }

    return (
        <>
            {indicator}
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
                            readReceipts={filteredReceipts}
                        />
                    </div>
                </FloatingFocusManager>
            )}
        </>
    );
}

export default memo(PostReadReceipts);