// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// markPostAsRead marks a post as read by the current user
func markPostAsRead(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequirePostId()
	if c.Err != nil {
		return
	}

	// Parse request body
	var readRequest model.ReadReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&readRequest); err != nil {
		c.SetInvalidParam("body")
		return
	}

	// Validate request
	if readRequest.PostId == "" {
		readRequest.PostId = c.Params.PostId
	}
	if readRequest.PostId != c.Params.PostId {
		c.SetInvalidParam("post_id")
		return
	}
	if err := readRequest.IsValid(); err != nil {
		c.Err = err
		return
	}

	// Check permissions - user must be able to read the channel
	if !c.App.SessionHasPermissionToChannelByPost(*c.AppContext.Session(), c.Params.PostId, model.PermissionReadChannelContent) {
		c.SetPermissionError(model.PermissionReadChannelContent)
		return
	}

	// Create and save the read receipt
	receipt, err := c.App.SaveReadReceiptForPost(c.AppContext, c.AppContext.Session().UserId, readRequest.PostId, readRequest.ReadAt, readRequest.DeviceId)
	if err != nil {
		c.Err = err
		return
	}

	// Return the created receipt
	if receiptJson, jsonErr := receipt.ToJSON(); jsonErr == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(receiptJson))
	} else {
		c.Err = model.NewAppError("markPostAsRead", "api.post.mark_read.marshal.app_error", nil, jsonErr.Error(), http.StatusInternalServerError)
	}
}

// unmarkPostAsRead removes a read receipt for a post (privacy feature)
func unmarkPostAsRead(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequirePostId()
	if c.Err != nil {
		return
	}

	// Check permissions - user must be able to read the channel
	if !c.App.SessionHasPermissionToChannelByPost(*c.AppContext.Session(), c.Params.PostId, model.PermissionReadChannelContent) {
		c.SetPermissionError(model.PermissionReadChannelContent)
		return
	}

	// Delete the read receipt
	err := c.App.DeleteReadReceiptForPost(c.AppContext, c.AppContext.Session().UserId, c.Params.PostId)
	if err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}

// getPostReadReceipts gets all read receipts for a specific post
func getPostReadReceipts(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequirePostId()
	if c.Err != nil {
		return
	}

	// Check permissions - user must be able to read the channel
	if !c.App.SessionHasPermissionToChannelByPost(*c.AppContext.Session(), c.Params.PostId, model.PermissionReadChannelContent) {
		c.SetPermissionError(model.PermissionReadChannelContent)
		return
	}

	// Parse query parameters
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"

	// Get read receipt info for the post
	receiptInfo, err := c.App.GetReadReceiptInfoForPost(c.AppContext, c.Params.PostId, c.AppContext.Session().UserId, includeDeleted)
	if err != nil {
		c.Err = err
		return
	}

	// Return the receipt info
	if receiptJson, jsonErr := receiptInfo.ToJSON(); jsonErr == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(receiptJson))
	} else {
		c.Err = model.NewAppError("getPostReadReceipts", "api.post.get_receipts.marshal.app_error", nil, jsonErr.Error(), http.StatusInternalServerError)
	}
}

// markPostsAsReadBatch marks multiple posts as read in a single request
func markPostsAsReadBatch(c *Context, w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var batchRequest model.ReadReceiptBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&batchRequest); err != nil {
		c.SetInvalidParam("body")
		return
	}

	// Validate request
	if err := batchRequest.IsValid(); err != nil {
		c.Err = err
		return
	}

	// Check permissions for all posts
	for _, postId := range batchRequest.PostIds {
		if !c.App.SessionHasPermissionToChannelByPost(*c.AppContext.Session(), postId, model.PermissionReadChannelContent) {
			c.SetPermissionError(model.PermissionReadChannelContent)
			return
		}
	}

	// Process batch read receipts
	receipts, err := c.App.SaveReadReceiptBatch(c.AppContext, c.AppContext.Session().UserId, &batchRequest)
	if err != nil {
		c.Err = err
		return
	}

	// Return success with count
	result := map[string]interface{}{
		"processed_count": len(receipts),
		"receipts":        receipts,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// getChannelReadReceiptSummary gets read receipt summary for a channel
func getChannelReadReceiptSummary(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireChannelId().RequireUserId()
	if c.Err != nil {
		return
	}

	// Check permissions - user must be able to read the channel
	if !c.App.SessionHasPermissionToChannel(*c.AppContext.Session(), c.Params.ChannelId, model.PermissionReadChannelContent) {
		c.SetPermissionError(model.PermissionReadChannelContent)
		return
	}

	// Check that requesting user matches the URL parameter (privacy)
	if !c.App.SessionHasPermissionToUser(*c.AppContext.Session(), c.Params.UserId) {
		c.SetPermissionError(model.PermissionEditOtherUsers)
		return
	}

	// Parse query parameters
	since := int64(0)
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if sinceInt, parseErr := strconv.ParseInt(sinceParam, 10, 64); parseErr == nil {
			since = sinceInt
		}
	}

	// Get channel read receipt summaries
	summaries, err := c.App.GetChannelReadReceiptSummary(c.AppContext, c.Params.ChannelId, c.Params.UserId, since)
	if err != nil {
		c.Err = err
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

// getUserReadReceiptHistory gets read receipt history for a user
func getUserReadReceiptHistory(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireUserId()
	if c.Err != nil {
		return
	}

	// Check permissions - users can only see their own read receipt history
	if !c.App.SessionHasPermissionToUser(*c.AppContext.Session(), c.Params.UserId) {
		c.SetPermissionError(model.PermissionEditOtherUsers)
		return
	}

	// Parse query parameters
	channelId := r.URL.Query().Get("channel_id")
	limitParam := r.URL.Query().Get("limit")
	limit := 100 // default limit
	if limitParam != "" {
		if limitInt, parseErr := strconv.Atoi(limitParam); parseErr == nil && limitInt > 0 && limitInt <= 1000 {
			limit = limitInt
		}
	}

	since := int64(0)
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if sinceInt, parseErr := strconv.ParseInt(sinceParam, 10, 64); parseErr == nil {
			since = sinceInt
		}
	}

	// Get user's read receipt history
	receipts, err := c.App.GetUserReadReceiptHistory(c.AppContext, c.Params.UserId, channelId, since, limit)
	if err != nil {
		c.Err = err
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(receipts)
}