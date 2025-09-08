// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {connect} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';
import type {Post} from '@mattermost/types/posts';

import {
    makeGetPostReadReceiptsWithProfiles,
    isPostReadReceiptsEnabled,
} from 'mattermost-redux/selectors/entities/posts';
import {getCurrentUserId, getUsers} from 'mattermost-redux/selectors/entities/users';
import {getChannel} from 'mattermost-redux/selectors/entities/channels';

import PostReadReceipts from './post_read_receipts';

type OwnProps = {
    postId: Post['id'];
    channelId: string;
}

function makeMapStateToProps() {
    const getPostReadReceiptsWithProfiles = makeGetPostReadReceiptsWithProfiles();
    
    return (state: GlobalState, ownProps: OwnProps) => {
        const post = state.entities.posts.posts[ownProps.postId];
        const channel = getChannel(state, ownProps.channelId);
        const readReceipts = getPostReadReceiptsWithProfiles(state, ownProps.postId);
        const isReadReceiptsEnabled = isPostReadReceiptsEnabled(state);
        
        const result = {
            currentUserId: getCurrentUserId(state),
            authorId: post?.user_id || '',
            channelType: channel?.type || '',
            readReceipts,
            isReadReceiptsEnabled,
            isDeleted: Boolean(post?.delete_at),
        };
        
        // Debug logging - track what data is being passed to PostReadReceipts
        console.log('READ_RECEIPTS_CONTAINER_DEBUG:', {
            postId: ownProps.postId,
            channelId: ownProps.channelId,
            channelType: result.channelType,
            isReadReceiptsEnabled: result.isReadReceiptsEnabled,
            readReceiptsCount: result.readReceipts?.length || 0,
            currentUserId: result.currentUserId,
            authorId: result.authorId,
            isDeleted: result.isDeleted,
            hasChannel: !!channel,
            hasPost: !!post,
        });
        
        return result;
    };
}

export default connect(makeMapStateToProps)(PostReadReceipts);