// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/store"
)

// SaveReadReceiptForPost creates or updates a read receipt for a post
func (a *App) SaveReadReceiptForPost(rctx request.CTX, userId, postId string, readAt int64, deviceId string) (*model.PostReadReceipt, *model.AppError) {
	mlog.Info("Starting read receipt save operation", 
		mlog.String("post_id", postId), 
		mlog.String("user_id", userId), 
		mlog.String("device_id", deviceId),
		mlog.Int64("read_at", readAt))
	
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		mlog.Warn("Read receipts feature is disabled", 
			mlog.String("post_id", postId), 
			mlog.String("user_id", userId))
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Get the post and validate
	post, err := a.GetSinglePost(rctx, postId, false)
	if err != nil {
		return nil, err
	}

	// 3. Validate channel type based on configuration
	channel, err := a.GetChannel(rctx, post.ChannelId)
	if err != nil {
		return nil, err
	}

	// Allow DM and GM channels always, team channels only if enabled
	allowedChannelType := channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup
	if !allowedChannelType && *a.Config().ServiceSettings.ReadReceiptsEnableTeamChannels {
		allowedChannelType = channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate
	}

	if !allowedChannelType {
		mlog.Warn("Read receipts not allowed for this channel type", 
			mlog.String("post_id", postId), 
			mlog.String("user_id", userId),
			mlog.String("channel_id", post.ChannelId),
			mlog.String("channel_type", string(channel.Type)),
			mlog.Bool("team_channels_enabled", *a.Config().ServiceSettings.ReadReceiptsEnableTeamChannels))
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.channel_type.app_error", nil, "", http.StatusBadRequest)
	}

	// 4. Check user's read receipt settings
	userSettings, err := a.GetUserReadReceiptSettings(rctx, userId)
	if err != nil {
		return nil, err
	}

	if userSettings.ReceiptMode == model.ReadReceiptModeDisabled {
		mlog.Warn("User has disabled read receipts", 
			mlog.String("post_id", postId), 
			mlog.String("user_id", userId),
			mlog.String("receipt_mode", userSettings.ReceiptMode))
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.user_disabled.app_error", nil, "", http.StatusForbidden)
	}

	// 5. Create the read receipt
	receipt := &model.PostReadReceipt{
		PostId:    postId,
		UserId:    userId,
		ChannelId: post.ChannelId,
		ReadAt:    readAt,
		DeviceId:  deviceId,
	}

	// Set device type based on device ID or session
	receipt.DeviceType = a.DetectDeviceType(rctx, deviceId)

	// Validate the receipt
	if validationErr := receipt.IsValid(); validationErr != nil {
		return nil, validationErr
	}

	// 6. Save to store
	mlog.Debug("Saving read receipt to database", 
		mlog.String("post_id", postId), 
		mlog.String("user_id", userId))
		
	savedReceipt, err := a.Srv().Store.PostReadReceipt().SaveReadReceipt(rctx, receipt)
	if err != nil {
		mlog.Error("Failed to save read receipt", 
			mlog.String("post_id", postId), 
			mlog.String("user_id", userId),
			mlog.Err(err))
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 7. Send websocket event for real-time updates
	mlog.Debug("Publishing read receipt WebSocket event", 
		mlog.String("post_id", postId), 
		mlog.String("user_id", userId))
	a.PublishReadReceiptEvent(rctx, savedReceipt, model.WebsocketEventPostRead)

	// 8. Update channel read receipt summary if needed
	mlog.Debug("Triggering async summary update", 
		mlog.String("post_id", postId), 
		mlog.String("channel_id", savedReceipt.ChannelId))
	go a.UpdateReadReceiptSummaryAsync(savedReceipt.ChannelId, savedReceipt.PostId)

	mlog.Info("Read receipt save operation completed successfully", 
		mlog.String("post_id", postId), 
		mlog.String("user_id", userId))

	return savedReceipt, nil
}

// SaveReadReceiptBatch processes multiple read receipts in a single operation
func (a *App) SaveReadReceiptBatch(rctx request.CTX, userId string, batchRequest *model.ReadReceiptBatchRequest) ([]*model.PostReadReceipt, *model.AppError) {
	mlog.Info("Starting batch read receipt save operation", 
		mlog.String("user_id", userId), 
		mlog.String("channel_id", batchRequest.ChannelId),
		mlog.Int("post_count", len(batchRequest.PostIds)),
		mlog.Int64("read_at", batchRequest.ReadAt))
	
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		mlog.Warn("Read receipts feature is disabled for batch operation", 
			mlog.String("user_id", userId),
			mlog.Int("post_count", len(batchRequest.PostIds)))
		return nil, model.NewAppError("SaveReadReceiptBatch", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Check user's read receipt settings
	userSettings, err := a.GetUserReadReceiptSettings(rctx, userId)
	if err != nil {
		return nil, err
	}

	if userSettings.ReceiptMode == model.ReadReceiptModeDisabled {
		mlog.Warn("User has disabled read receipts for batch operation", 
			mlog.String("user_id", userId),
			mlog.Int("post_count", len(batchRequest.PostIds)),
			mlog.String("receipt_mode", userSettings.ReceiptMode))
		return nil, model.NewAppError("SaveReadReceiptBatch", "app.post.read_receipt.user_disabled.app_error", nil, "", http.StatusForbidden)
	}

	// 3. Build batch data
	batch := &model.PostReadReceiptBatch{
		PostIds:   batchRequest.PostIds,
		UserId:    userId,
		ChannelId: batchRequest.ChannelId,
		ReadAt:    batchRequest.ReadAt,
		DeviceId:  batchRequest.DeviceId,
	}

	if batch.ReadAt == 0 {
		batch.ReadAt = model.GetMillis()
	}

	// 4. Validate all posts exist and are in valid channels
	posts, err := a.GetPostsByIds(rctx, batch.PostIds)
	if err != nil {
		return nil, err
	}

	var validatedReceipts []*model.PostReadReceipt
	channelIds := make(map[string]bool)

	for _, post := range posts {
		// Validate channel type based on configuration
		if channelIds[post.ChannelId] == false {
			channel, channelErr := a.GetChannel(rctx, post.ChannelId)
			if channelErr != nil {
				continue // Skip invalid posts
			}

			// Allow DM and GM channels always, team channels only if enabled
			allowedChannelType := channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup
			if !allowedChannelType && *a.Config().ServiceSettings.ReadReceiptsEnableTeamChannels {
				allowedChannelType = channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate
			}

			if !allowedChannelType {
				continue // Skip posts in unsupported channel types
			}

			channelIds[post.ChannelId] = true
		}

		receipt := &model.PostReadReceipt{
			PostId:     post.Id,
			UserId:     userId,
			ChannelId:  post.ChannelId,
			ReadAt:     batch.ReadAt,
			DeviceId:   batch.DeviceId,
			DeviceType: a.DetectDeviceType(rctx, batch.DeviceId),
		}

		if receipt.IsValid() == nil {
			validatedReceipts = append(validatedReceipts, receipt)
		}
	}

	// 5. Save batch to store
	mlog.Debug("Saving batch to database", 
		mlog.String("user_id", userId), 
		mlog.Int("validated_count", len(validatedReceipts)))
		
	err = a.Srv().Store.PostReadReceipt().SaveReadReceiptBatch(rctx, batch)
	if err != nil {
		mlog.Error("Failed to save batch read receipts", 
			mlog.String("user_id", userId),
			mlog.Int("post_count", len(batchRequest.PostIds)),
			mlog.Err(err))
		return nil, model.NewAppError("SaveReadReceiptBatch", "app.post.read_receipt.batch_save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 6. Send websocket events
	mlog.Debug("Publishing batch WebSocket events", 
		mlog.String("user_id", userId), 
		mlog.Int("receipt_count", len(validatedReceipts)))
	a.PublishReadReceiptBatchEvent(rctx, validatedReceipts, model.WebsocketEventPostReadBatch)

	mlog.Info("Batch read receipt save operation completed successfully", 
		mlog.String("user_id", userId), 
		mlog.Int("requested_count", len(batchRequest.PostIds)),
		mlog.Int("processed_count", len(validatedReceipts)))

	return validatedReceipts, nil
}

// GetReadReceiptInfoForPost gets comprehensive read receipt information for a post
func (a *App) GetReadReceiptInfoForPost(rctx request.CTX, postId, requestingUserId string, includeDeleted bool) (*model.PostReadReceiptInfo, *model.AppError) {
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return nil, model.NewAppError("GetReadReceiptInfoForPost", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Check if user has privacy permissions
	userSettings, err := a.GetUserReadReceiptSettings(rctx, requestingUserId)
	if err != nil {
		return nil, err
	}

	// 3. Get read receipt info from store
	info, err := a.Srv().Store.PostReadReceipt().GetReadReceiptInfo(postId)
	if err != nil {
		return nil, model.NewAppError("GetReadReceiptInfoForPost", "app.post.read_receipt.get_info.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 4. Apply privacy filtering based on user settings
	if userSettings.ShowOthersReceipts == model.ReadReceiptVisibilityNone {
		// User doesn't want to see others' receipts, filter to only their own
		filteredReceipts := []*model.PostReadReceipt{}
		for _, receipt := range info.ReadReceipts {
			if receipt.UserId == requestingUserId {
				filteredReceipts = append(filteredReceipts, receipt)
			}
		}
		info.ReadReceipts = filteredReceipts
		info.ReadCount = len(filteredReceipts)
	}

	return info, nil
}

// DeleteReadReceiptForPost removes a read receipt (privacy feature)
func (a *App) DeleteReadReceiptForPost(rctx request.CTX, userId, postId string) *model.AppError {
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return model.NewAppError("DeleteReadReceiptForPost", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Check if privacy deletion is allowed
	if !*a.Config().ServiceSettings.ReadReceiptsAllowPrivacyDeletion {
		return model.NewAppError("DeleteReadReceiptForPost", "app.post.read_receipt.privacy_deletion_disabled.app_error", nil, "", http.StatusForbidden)
	}

	// 3. Delete from store
	err := a.Srv().Store.PostReadReceipt().DeleteReadReceipt(postId, userId)
	if err != nil {
		return model.NewAppError("DeleteReadReceiptForPost", "app.post.read_receipt.delete.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 4. Create audit log entry
	audit := &model.ReadReceiptAuditLog{
		Id:       model.NewId(),
		UserId:   userId,
		PostId:   postId,
		Action:   model.ReadReceiptActionPrivacyView,
		Metadata: map[string]interface{}{
			"action": "delete_receipt",
			"reason": "user_privacy_request",
		},
		CreateAt: model.GetMillis(),
	}

	a.Srv().Store.PostReadReceipt().SaveReadReceiptAuditLog(audit)

	return nil
}

// GetChannelReadReceiptSummary gets read receipt summaries for a channel
func (a *App) GetChannelReadReceiptSummary(rctx request.CTX, channelId, userId string, since int64) ([]*model.PostReadReceiptSummary, *model.AppError) {
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return nil, model.NewAppError("GetChannelReadReceiptSummary", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Validate channel type based on configuration
	channel, err := a.GetChannel(rctx, channelId)
	if err != nil {
		return nil, err
	}

	// Allow DM and GM channels always, team channels only if enabled
	allowedChannelType := channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup
	if !allowedChannelType && *a.Config().ServiceSettings.ReadReceiptsEnableTeamChannels {
		allowedChannelType = channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate
	}

	if !allowedChannelType {
		return nil, model.NewAppError("GetChannelReadReceiptSummary", "app.post.read_receipt.channel_type.app_error", nil, "", http.StatusBadRequest)
	}

	// 3. Get summaries from store
	summaries, err := a.Srv().Store.PostReadReceipt().GetReadReceiptSummariesForChannel(channelId, since)
	if err != nil {
		return nil, model.NewAppError("GetChannelReadReceiptSummary", "app.post.read_receipt.get_summaries.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return summaries, nil
}

// GetUserReadReceiptHistory gets a user's read receipt history
func (a *App) GetUserReadReceiptHistory(rctx request.CTX, userId, channelId string, since int64, limit int) ([]*model.PostReadReceipt, *model.AppError) {
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return nil, model.NewAppError("GetUserReadReceiptHistory", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Get receipts from store
	receipts, err := a.Srv().Store.PostReadReceipt().GetReadReceiptsForUser(userId, channelId, limit)
	if err != nil {
		return nil, model.NewAppError("GetUserReadReceiptHistory", "app.post.read_receipt.get_user_receipts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	return receipts, nil
}

// Helper functions

// GetUserReadReceiptSettings gets a user's read receipt configuration
func (a *App) GetUserReadReceiptSettings(rctx request.CTX, userId string) (*model.UserReadReceiptSettings, *model.AppError) {
	// Get user preferences for read receipts
	preferences, err := a.GetPreferencesForUser(rctx, userId)
	if err != nil {
		return nil, err
	}

	settings := &model.UserReadReceiptSettings{
		ReceiptMode:        *a.Config().ServiceSettings.ReadReceiptsDefaultSetting,
		ShowOthersReceipts: model.ReadReceiptVisibilityAll,
	}

	// Override with user preferences if they exist
	for _, pref := range preferences {
		if pref.Category == model.PreferenceCategoryReadReceipts {
			switch pref.Name {
			case "receipt_mode":
				settings.ReceiptMode = pref.Value
			case "show_others_receipts":
				settings.ShowOthersReceipts = pref.Value
			}
		}
	}

	return settings, nil
}

// DetectDeviceType determines device type from device ID or session info
func (a *App) DetectDeviceType(rctx request.CTX, deviceId string) string {
	if deviceId == "" {
		return model.DeviceTypeUnknown
	}

	// Add logic to detect device type based on patterns or session info
	// This is a simplified implementation
	return model.DeviceTypeWeb
}

// PublishReadReceiptEvent sends websocket event for single read receipt
func (a *App) PublishReadReceiptEvent(rctx request.CTX, receipt *model.PostReadReceipt, event model.WebsocketEventType) {
	mlog.Debug("Publishing read receipt WebSocket event", 
		mlog.String("event_type", string(event)), 
		mlog.String("post_id", receipt.PostId), 
		mlog.String("user_id", receipt.UserId),
		mlog.String("channel_id", receipt.ChannelId))
	
	message := model.NewWebSocketEvent(event, "", receipt.ChannelId, "", nil, "")
	message.Add("post_id", receipt.PostId)
	message.Add("user_id", receipt.UserId)
	message.Add("read_at", receipt.ReadAt)

	a.Publish(message)
	
	mlog.Debug("Read receipt WebSocket event published successfully", 
		mlog.String("event_type", string(event)), 
		mlog.String("post_id", receipt.PostId), 
		mlog.String("user_id", receipt.UserId))
}

// PublishReadReceiptBatchEvent sends websocket event for batch read receipts
func (a *App) PublishReadReceiptBatchEvent(rctx request.CTX, receipts []*model.PostReadReceipt, event model.WebsocketEventType) {
	mlog.Debug("Publishing batch read receipt WebSocket event", 
		mlog.String("event_type", string(event)), 
		mlog.Int("total_receipts", len(receipts)))
	
	if len(receipts) == 0 {
		mlog.Debug("No receipts to publish, skipping WebSocket event")
		return
	}

	// Group by channel for efficient broadcasting
	channelGroups := make(map[string][]*model.PostReadReceipt)
	for _, receipt := range receipts {
		channelGroups[receipt.ChannelId] = append(channelGroups[receipt.ChannelId], receipt)
	}

	for channelId, channelReceipts := range channelGroups {
		mlog.Debug("Publishing batch event for channel", 
			mlog.String("channel_id", channelId), 
			mlog.Int("receipt_count", len(channelReceipts)))
		
		message := model.NewWebSocketEvent(event, "", channelId, "", nil, "")
		message.Add("receipts", channelReceipts)
		message.Add("count", len(channelReceipts))

		a.Publish(message)
	}
	
	mlog.Debug("Batch read receipt WebSocket events published successfully", 
		mlog.String("event_type", string(event)), 
		mlog.Int("channel_count", len(channelGroups)),
		mlog.Int("total_receipts", len(receipts)))
}

// UpdateReadReceiptSummaryAsync updates channel summary asynchronously
func (a *App) UpdateReadReceiptSummaryAsync(channelId, postId string) {
	go func() {
		// This would typically use a job queue, but for simplicity we'll do it inline
		// In production, this should be queued to prevent blocking
		rctx := request.EmptyContext(mlog.CreateConsoleTestLogger())
		
		summary, err := a.Srv().Store.PostReadReceipt().GetReadReceiptSummary(postId)
		if err != nil {
			mlog.Warn("Failed to get read receipt summary for update", mlog.String("post_id", postId), mlog.Err(err))
			return
		}

		summary.LastUpdated = model.GetMillis()
		
		if updateErr := a.Srv().Store.PostReadReceipt().UpdateReadReceiptSummary(summary); updateErr != nil {
			mlog.Warn("Failed to update read receipt summary", mlog.String("post_id", postId), mlog.Err(updateErr))
		}
	}()
}

// BackfillReadReceiptsForChannel creates read receipts for historical posts based on channel view times
func (a *App) BackfillReadReceiptsForChannel(rctx request.CTX, channelId string) *model.AppError {
	mlog.Info("Starting read receipts backfill for channel", 
		mlog.String("channel_id", channelId))
	
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return model.NewAppError("BackfillReadReceiptsForChannel", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Get channel and validate type
	channel, err := a.GetChannel(rctx, channelId)
	if err != nil {
		return err
	}

	// Allow DM and GM channels always, team channels only if enabled
	allowedChannelType := channel.Type == model.ChannelTypeDirect || channel.Type == model.ChannelTypeGroup
	if !allowedChannelType && *a.Config().ServiceSettings.ReadReceiptsEnableTeamChannels {
		allowedChannelType = channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate
	}

	if !allowedChannelType {
		mlog.Warn("Backfill not allowed for this channel type", 
			mlog.String("channel_id", channelId),
			mlog.String("channel_type", string(channel.Type)))
		return model.NewAppError("BackfillReadReceiptsForChannel", "app.post.read_receipt.channel_type.app_error", nil, "", http.StatusBadRequest)
	}

	// 3. Get all channel members with their last viewed times
	members, err := a.GetChannelMembers(rctx, channelId, 0, 200)
	if err != nil {
		return err
	}

	// 4. Get all posts in the channel (recent ones)
	postList, err := a.GetPostsForChannel(rctx, channelId, 0, 60) // Last 60 posts
	if err != nil {
		return err
	}

	mlog.Info("Backfilling read receipts", 
		mlog.String("channel_id", channelId),
		mlog.Int("member_count", len(members)),
		mlog.Int("post_count", len(postList.Posts)))

	var receiptsToCreate []*model.PostReadReceipt
	currentTime := model.GetMillis()

	// 5. For each member, check which posts they would have "read"
	for _, member := range members {
		// Skip if no last viewed time
		if member.LastViewedAt == 0 {
			continue
		}

		for _, post := range postList.Posts {
			// Skip if post is after user's last view time
			if post.CreateAt > member.LastViewedAt {
				continue
			}

			// Skip own posts
			if post.UserId == member.UserId {
				continue
			}

			// Check if read receipt already exists
			existing, existErr := a.Srv().Store.PostReadReceipt().GetReadReceipt(post.Id, member.UserId)
			if existErr == nil && existing != nil {
				continue // Already has read receipt
			}

			// Create read receipt with the user's last viewed time
			receipt := &model.PostReadReceipt{
				PostId:     post.Id,
				UserId:     member.UserId,
				ChannelId:  channelId,
				ReadAt:     member.LastViewedAt,
				DeviceId:   "backfill",
				DeviceType: "backfill",
			}

			receiptsToCreate = append(receiptsToCreate, receipt)
		}
	}

	mlog.Info("Creating backfill read receipts", 
		mlog.String("channel_id", channelId),
		mlog.Int("receipts_to_create", len(receiptsToCreate)))

	// 6. Batch create the read receipts
	for _, receipt := range receiptsToCreate {
		_, saveErr := a.Srv().Store.PostReadReceipt().SaveReadReceipt(rctx, receipt)
		if saveErr != nil {
			mlog.Warn("Failed to save backfill read receipt", 
				mlog.String("post_id", receipt.PostId),
				mlog.String("user_id", receipt.UserId),
				mlog.Err(saveErr))
			// Continue with other receipts
		}
	}

	mlog.Info("Read receipts backfill completed", 
		mlog.String("channel_id", channelId),
		mlog.Int("receipts_created", len(receiptsToCreate)))

	return nil
}