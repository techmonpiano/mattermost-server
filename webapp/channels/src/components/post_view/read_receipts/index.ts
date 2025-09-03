// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import {connect} from 'react-redux';

import type {Post} from '@mattermost/types/posts';

import {getCurrentUserId} from 'mattermost-redux/selectors/entities/common';
import {getHasReactions, makeGetPostReadReceiptsWithProfiles} from 'mattermost-redux/selectors/entities/posts';

import type {GlobalState} from 'types/store';

import PostReadReceipts from './post_read_receipts';

type OwnProps = {
    postId: Post['id'];
};

function makeMapStateToProps() {
    const getPostReadReceiptsWithProfiles = makeGetPostReadReceiptsWithProfiles();

    return (state: GlobalState, ownProps: OwnProps) => {
        const currentUserId = getCurrentUserId(state);
        const hasReactions = getHasReactions(state, ownProps.postId);
        const list = getPostReadReceiptsWithProfiles(state, ownProps.postId);

        return {
            currentUserId,
            hasReactions,
            list,
        };
    };
}

export default connect(makeMapStateToProps)(PostReadReceipts);