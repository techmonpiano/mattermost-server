// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import React from 'react';
import {FormattedMessage, FormattedTime} from 'react-intl';

import type {UserProfile} from '@mattermost/types/users';

import Avatar from 'components/widgets/users/avatar';
import {getFullName} from 'mattermost-redux/utils/user_utils';

import type {ReadReceiptData} from './post_read_receipts';

type Props = {
    currentUserId: UserProfile['id'];
    readReceipts: ReadReceiptData[];
}

export default function PostReadReceiptsUserPopover({
    currentUserId,
    readReceipts,
}: Props) {
    // Sort by read time (most recent first)
    const sortedReceipts = [...readReceipts].sort((a, b) => b.readAt - a.readAt);
    
    return (
        <div className='ReadReceiptPopover'>
            <div className='ReadReceiptPopover__header'>
                <FormattedMessage
                    id='post_read_receipts.popover.header'
                    defaultMessage='Read by'
                />
            </div>
            <div className='ReadReceiptPopover__body'>
                {sortedReceipts.map((receipt) => {
                    const user = receipt.user;
                    const displayName = getFullName(user) || user.username;
                    
                    return (
                        <div
                            key={user.id}
                            className='ReadReceiptPopover__user'
                        >
                            <Avatar
                                size='sm'
                                userId={user.id}
                                username={user.username}
                            />
                            <div className='ReadReceiptPopover__user-info'>
                                <div className='ReadReceiptPopover__user-name'>
                                    {displayName}
                                </div>
                                <div className='ReadReceiptPopover__read-time'>
                                    <FormattedTime
                                        value={receipt.readAt}
                                        hour='numeric'
                                        minute='2-digit'
                                        hour12={true}
                                    />
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}