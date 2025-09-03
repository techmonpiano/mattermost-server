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
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Get the post and validate
	post, err := a.GetSinglePost(rctx, postId, false)
	if err != nil {
		return nil, err
	}

	// 3. Validate this is a DM/GM channel (read receipts only for DMs/GMs)
	channel, err := a.GetChannel(rctx, post.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.channel_type.app_error", nil, "", http.StatusBadRequest)
	}

	// 4. Check user's read receipt settings
	userSettings, err := a.GetUserReadReceiptSettings(rctx, userId)
	if err != nil {
		return nil, err
	}

	if userSettings.ReceiptMode == model.ReadReceiptModeDisabled {
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
	savedReceipt, err := a.Srv().Store.PostReadReceipt().SaveReadReceipt(rctx, receipt)
	if err != nil {
		return nil, model.NewAppError("SaveReadReceiptForPost", "app.post.read_receipt.save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 7. Send websocket event for real-time updates
	a.PublishReadReceiptEvent(rctx, savedReceipt, model.WebsocketEventPostRead)

	// 8. Update channel read receipt summary if needed
	go a.UpdateReadReceiptSummaryAsync(savedReceipt.ChannelId, savedReceipt.PostId)

	return savedReceipt, nil
}

// SaveReadReceiptBatch processes multiple read receipts in a single operation
func (a *App) SaveReadReceiptBatch(rctx request.CTX, userId string, batchRequest *model.ReadReceiptBatchRequest) ([]*model.PostReadReceipt, *model.AppError) {
	// 1. Validate read receipts are enabled
	if !*a.Config().ServiceSettings.EnableReadReceipts {
		return nil, model.NewAppError("SaveReadReceiptBatch", "app.post.read_receipt.disabled.app_error", nil, "", http.StatusNotImplemented)
	}

	// 2. Check user's read receipt settings
	userSettings, err := a.GetUserReadReceiptSettings(rctx, userId)
	if err != nil {
		return nil, err
	}

	if userSettings.ReceiptMode == model.ReadReceiptModeDisabled {
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
		// Validate channel type (DM/GM only)
		if channelIds[post.ChannelId] == false {
			channel, channelErr := a.GetChannel(rctx, post.ChannelId)
			if channelErr != nil {
				continue // Skip invalid posts
			}

			if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
				continue // Skip posts not in DMs/GMs
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
	err = a.Srv().Store.PostReadReceipt().SaveReadReceiptBatch(rctx, batch)
	if err != nil {
		return nil, model.NewAppError("SaveReadReceiptBatch", "app.post.read_receipt.batch_save.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// 6. Send websocket events
	a.PublishReadReceiptBatchEvent(rctx, validatedReceipts, model.WebsocketEventPostReadBatch)

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

	// 2. Validate channel type (DM/GM only)
	channel, err := a.GetChannel(rctx, channelId)
	if err != nil {
		return nil, err
	}

	if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
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
	message := model.NewWebSocketEvent(event, "", receipt.ChannelId, "", nil, "")
	message.Add("post_id", receipt.PostId)
	message.Add("user_id", receipt.UserId)
	message.Add("read_at", receipt.ReadAt)

	a.Publish(message)
}

// PublishReadReceiptBatchEvent sends websocket event for batch read receipts
func (a *App) PublishReadReceiptBatchEvent(rctx request.CTX, receipts []*model.PostReadReceipt, event model.WebsocketEventType) {
	if len(receipts) == 0 {
		return
	}

	// Group by channel for efficient broadcasting
	channelGroups := make(map[string][]*model.PostReadReceipt)
	for _, receipt := range receipts {
		channelGroups[receipt.ChannelId] = append(channelGroups[receipt.ChannelId], receipt)
	}

	for channelId, channelReceipts := range channelGroups {
		message := model.NewWebSocketEvent(event, "", channelId, "", nil, "")
		message.Add("receipts", channelReceipts)
		message.Add("count", len(channelReceipts))

		a.Publish(message)
	}
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