// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {PostReadReceipt, ReadReceiptBatchRequest, PostReadReceiptInfo, PostReadReceiptSummary} from '@mattermost/types/posts';
import type {StatusOK} from '@mattermost/types/client4';

import {
    Client4 as ClientClass4,
    DEFAULT_LIMIT_AFTER,
    DEFAULT_LIMIT_BEFORE,
} from '@mattermost/client';

const Client4 = new ClientClass4();

// Extend Client4 with read receipt methods
Client4.markPostAsRead = function(postId: string, userId: string, readAt?: number, deviceId?: string) {
    this.trackEvent('api', 'api_posts_read_receipt');
    
    const body: any = {
        post_id: postId,
        read_at: readAt || Date.now(),
    };
    
    if (deviceId) {
        body.device_id = deviceId;
    }
    
    return this.doFetch<PostReadReceipt>(
        `${this.getPostRoute(postId)}/read`,
        {method: 'post', body: JSON.stringify(body)},
    );
};

Client4.unmarkPostAsRead = function(postId: string, userId: string) {
    return this.doFetch<null>(
        `${this.getPostRoute(postId)}/read`,
        {method: 'delete'},
    );
};

Client4.getPostReadReceipts = function(postId: string, includeDeleted: boolean = false) {
    const params = new URLSearchParams();
    if (includeDeleted) {
        params.set('include_deleted', 'true');
    }
    
    return this.doFetch<PostReadReceiptInfo>(
        `${this.getPostRoute(postId)}/receipts?${params.toString()}`,
        {method: 'get'},
    );
};

Client4.markPostsAsReadBatch = function(postIds: string[], channelId: string, readAt?: number, deviceId?: string) {
    this.trackEvent('api', 'api_posts_read_receipt_batch');
    
    const body: ReadReceiptBatchRequest = {
        post_ids: postIds,
        channel_id: channelId,
        read_at: readAt || Date.now(),
        device_id: deviceId,
    };
    
    return this.doFetch<{processed_count: number; receipts: PostReadReceipt[]}>(
        `${this.getPostsRoute()}/read/batch`,
        {method: 'post', body: JSON.stringify(body)},
    );
};

Client4.getChannelReadReceiptSummary = function(channelId: string, userId: string, since?: number) {
    const params = new URLSearchParams();
    if (since) {
        params.set('since', since.toString());
    }
    
    return this.doFetch<PostReadReceiptSummary[]>(
        `${this.getUserRoute(userId)}/channels/${channelId}/read_receipts?${params.toString()}`,
        {method: 'get'},
    );
};

Client4.getUserReadReceiptHistory = function(userId: string, channelId?: string, since?: number, limit?: number) {
    const params = new URLSearchParams();
    if (channelId) {
        params.set('channel_id', channelId);
    }
    if (since) {
        params.set('since', since.toString());
    }
    if (limit) {
        params.set('limit', limit.toString());
    }
    
    return this.doFetch<PostReadReceipt[]>(
        `${this.getUserRoute(userId)}/read_receipts?${params.toString()}`,
        {method: 'get'},
    );
};

export {
    Client4,
    DEFAULT_LIMIT_AFTER,
    DEFAULT_LIMIT_BEFORE,
};
