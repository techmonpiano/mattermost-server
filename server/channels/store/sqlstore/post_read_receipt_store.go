// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	sq "github.com/mattermost/squirrel"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/public/shared/request"
	"github.com/mattermost/mattermost/server/v8/channels/store"
)

type SqlPostReadReceiptStore struct {
	*SqlStore
}

func newSqlPostReadReceiptStore(sqlStore *SqlStore) store.PostReadReceiptStore {
	return &SqlPostReadReceiptStore{sqlStore}
}

// Core read receipt operations

func (s *SqlPostReadReceiptStore) SaveReadReceipt(rctx request.CTX, receipt *model.PostReadReceipt) (*model.PostReadReceipt, error) {
	mlog.Debug("Saving read receipt to database", 
		mlog.String("post_id", receipt.PostId), 
		mlog.String("user_id", receipt.UserId),
		mlog.String("channel_id", receipt.ChannelId))
	
	receipt.PreSave()

	query := s.getQueryBuilder().
		Insert("PostReadReceipts").
		Columns("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		Values(receipt.PostId, receipt.UserId, receipt.ChannelId, receipt.ReadAt, receipt.CreateAt, receipt.DeviceId, receipt.DeviceType, receipt.SessionId)

	// Use ON DUPLICATE KEY UPDATE for MySQL or UPSERT for PostgreSQL
	if s.DriverName() == model.DatabaseDriverPostgres {
		query = query.Suffix("ON CONFLICT (PostId, UserId) DO UPDATE SET ReadAt = ?, CreateAt = ?, DeviceId = ?, DeviceType = ?, SessionId = ?",
			receipt.ReadAt, receipt.CreateAt, receipt.DeviceId, receipt.DeviceType, receipt.SessionId)
	} else {
		query = query.Suffix("ON DUPLICATE KEY UPDATE ReadAt = ?, CreateAt = ?, DeviceId = ?, DeviceType = ?, SessionId = ?",
			receipt.ReadAt, receipt.CreateAt, receipt.DeviceId, receipt.DeviceType, receipt.SessionId)
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		mlog.Error("Failed to build read receipt save query", 
			mlog.String("post_id", receipt.PostId), 
			mlog.String("user_id", receipt.UserId),
			mlog.Err(err))
		return nil, errors.Wrap(err, "save_read_receipt_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		mlog.Error("Failed to execute read receipt save query", 
			mlog.String("post_id", receipt.PostId), 
			mlog.String("user_id", receipt.UserId),
			mlog.Err(err))
		return nil, errors.Wrap(err, "save_read_receipt")
	}

	mlog.Debug("Read receipt saved successfully", 
		mlog.String("post_id", receipt.PostId), 
		mlog.String("user_id", receipt.UserId))

	return receipt, nil
}

func (s *SqlPostReadReceiptStore) SaveReadReceiptBatch(rctx request.CTX, batch *model.PostReadReceiptBatch) error {
	mlog.Debug("Processing batch read receipt save", 
		mlog.String("user_id", batch.UserId), 
		mlog.String("channel_id", batch.ChannelId),
		mlog.Int("post_count", len(batch.PostIds)))
	
	if len(batch.PostIds) == 0 {
		mlog.Debug("Empty batch read receipt request, skipping")
		return nil
	}

	// Build batch insert
	query := s.getQueryBuilder().Insert("PostReadReceipts").
		Columns("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId")

	createAt := model.GetMillis()
	deviceType := model.DeviceTypeWeb
	if batch.DeviceId != "" {
		// Simple device type detection - could be enhanced
		if strings.Contains(batch.DeviceId, "mobile") {
			deviceType = model.DeviceTypeMobile
		} else if strings.Contains(batch.DeviceId, "desktop") {
			deviceType = model.DeviceTypeDesktop
		}
	}

	for _, postId := range batch.PostIds {
		query = query.Values(postId, batch.UserId, batch.ChannelId, batch.ReadAt, createAt, batch.DeviceId, deviceType, "")
	}

	// Handle conflicts
	if s.DriverName() == model.DatabaseDriverPostgres {
		query = query.Suffix("ON CONFLICT (PostId, UserId) DO UPDATE SET ReadAt = EXCLUDED.ReadAt, CreateAt = EXCLUDED.CreateAt")
	} else {
		query = query.Suffix("ON DUPLICATE KEY UPDATE ReadAt = VALUES(ReadAt), CreateAt = VALUES(CreateAt)")
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		mlog.Error("Failed to build batch read receipt save query", 
			mlog.String("user_id", batch.UserId), 
			mlog.String("channel_id", batch.ChannelId),
			mlog.Int("post_count", len(batch.PostIds)),
			mlog.Err(err))
		return errors.Wrap(err, "save_read_receipt_batch_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		mlog.Error("Failed to execute batch read receipt save query", 
			mlog.String("user_id", batch.UserId), 
			mlog.String("channel_id", batch.ChannelId),
			mlog.Int("post_count", len(batch.PostIds)),
			mlog.Err(err))
		return errors.Wrap(err, "save_read_receipt_batch")
	}

	mlog.Debug("Batch read receipts saved successfully", 
		mlog.String("user_id", batch.UserId), 
		mlog.String("channel_id", batch.ChannelId),
		mlog.Int("post_count", len(batch.PostIds)))
	
	return nil
}

func (s *SqlPostReadReceiptStore) GetReadReceipt(postID, userID string) (*model.PostReadReceipt, error) {
	query := s.getQueryBuilder().
		Select("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		From("PostReadReceipts").
		Where(sq.And{
			sq.Eq{"PostId": postID},
			sq.Eq{"UserId": userID},
		})

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_tosql")
	}

	var receipt model.PostReadReceipt
	if err := s.GetReplica().Get(&receipt, queryString, args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, store.NewErrNotFound("PostReadReceipt", fmt.Sprintf("postId=%s, userId=%s", postID, userID))
		}
		return nil, errors.Wrapf(err, "get_read_receipt postId=%s userId=%s", postID, userID)
	}

	return &receipt, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptsForPost(postID string, includeDeleted bool) ([]*model.PostReadReceipt, error) {
	mlog.Debug("Getting read receipts for post", 
		mlog.String("post_id", postID), 
		mlog.Bool("include_deleted", includeDeleted))
	
	query := s.getQueryBuilder().
		Select("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		From("PostReadReceipts").
		Where(sq.Eq{"PostId": postID}).
		OrderBy("ReadAt DESC")

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipts_for_post_tosql")
	}

	var receipts []*model.PostReadReceipt
	if err := s.GetReplica().Select(&receipts, queryString, args...); err != nil {
		mlog.Error("Failed to get read receipts for post", 
			mlog.String("post_id", postID),
			mlog.Err(err))
		return nil, errors.Wrapf(err, "get_read_receipts_for_post postId=%s", postID)
	}

	mlog.Debug("Retrieved read receipts for post", 
		mlog.String("post_id", postID), 
		mlog.Int("receipt_count", len(receipts)))

	return receipts, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptsForPosts(postIDs []string) (map[string][]*model.PostReadReceipt, error) {
	if len(postIDs) == 0 {
		return make(map[string][]*model.PostReadReceipt), nil
	}

	query := s.getQueryBuilder().
		Select("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		From("PostReadReceipts").
		Where(sq.Eq{"PostId": postIDs}).
		OrderBy("PostId", "ReadAt DESC")

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipts_for_posts_tosql")
	}

	var receipts []*model.PostReadReceipt
	if err := s.GetReplica().Select(&receipts, queryString, args...); err != nil {
		return nil, errors.Wrap(err, "get_read_receipts_for_posts")
	}

	// Group by post ID
	result := make(map[string][]*model.PostReadReceipt)
	for _, receipt := range receipts {
		result[receipt.PostId] = append(result[receipt.PostId], receipt)
	}

	return result, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptsForUser(userID string, channelID string, limit int) ([]*model.PostReadReceipt, error) {
	mlog.Debug("Getting read receipts for user", 
		mlog.String("user_id", userID), 
		mlog.String("channel_id", channelID),
		mlog.Int("limit", limit))
	
	query := s.getQueryBuilder().
		Select("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		From("PostReadReceipts").
		Where(sq.Eq{"UserId": userID}).
		OrderBy("ReadAt DESC").
		Limit(uint64(limit))

	if channelID != "" {
		query = query.Where(sq.Eq{"ChannelId": channelID})
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipts_for_user_tosql")
	}

	var receipts []*model.PostReadReceipt
	if err := s.GetReplica().Select(&receipts, queryString, args...); err != nil {
		mlog.Error("Failed to get read receipts for user", 
			mlog.String("user_id", userID),
			mlog.String("channel_id", channelID),
			mlog.Err(err))
		return nil, errors.Wrapf(err, "get_read_receipts_for_user userId=%s", userID)
	}

	mlog.Debug("Retrieved read receipts for user", 
		mlog.String("user_id", userID), 
		mlog.String("channel_id", channelID),
		mlog.Int("receipt_count", len(receipts)))

	return receipts, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptsForChannel(channelID string, since int64) ([]*model.PostReadReceipt, error) {
	query := s.getQueryBuilder().
		Select("PostId", "UserId", "ChannelId", "ReadAt", "CreateAt", "DeviceId", "DeviceType", "SessionId").
		From("PostReadReceipts").
		Where(sq.Eq{"ChannelId": channelID}).
		OrderBy("ReadAt DESC")

	if since > 0 {
		query = query.Where(sq.GtOrEq{"ReadAt": since})
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipts_for_channel_tosql")
	}

	var receipts []*model.PostReadReceipt
	if err := s.GetReplica().Select(&receipts, queryString, args...); err != nil {
		return nil, errors.Wrapf(err, "get_read_receipts_for_channel channelId=%s", channelID)
	}

	return receipts, nil
}

// Delete operations

func (s *SqlPostReadReceiptStore) DeleteReadReceipt(postID, userID string) error {
	mlog.Debug("Deleting read receipt", 
		mlog.String("post_id", postID), 
		mlog.String("user_id", userID))
	
	query := s.getQueryBuilder().
		Delete("PostReadReceipts").
		Where(sq.And{
			sq.Eq{"PostId": postID},
			sq.Eq{"UserId": userID},
		})

	queryString, args, err := query.ToSql()
	if err != nil {
		mlog.Error("Failed to build delete read receipt query", 
			mlog.String("post_id", postID), 
			mlog.String("user_id", userID),
			mlog.Err(err))
		return errors.Wrap(err, "delete_read_receipt_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		mlog.Error("Failed to execute delete read receipt query", 
			mlog.String("post_id", postID), 
			mlog.String("user_id", userID),
			mlog.Err(err))
		return errors.Wrapf(err, "delete_read_receipt postId=%s userId=%s", postID, userID)
	}

	mlog.Debug("Read receipt deleted successfully", 
		mlog.String("post_id", postID), 
		mlog.String("user_id", userID))

	return nil
}

func (s *SqlPostReadReceiptStore) DeleteReadReceiptsForUser(userID string) error {
	query := s.getQueryBuilder().
		Delete("PostReadReceipts").
		Where(sq.Eq{"UserId": userID})

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "delete_read_receipts_for_user_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrapf(err, "delete_read_receipts_for_user userId=%s", userID)
	}

	return nil
}

func (s *SqlPostReadReceiptStore) DeleteReadReceiptsForPost(postID string) error {
	query := s.getQueryBuilder().
		Delete("PostReadReceipts").
		Where(sq.Eq{"PostId": postID})

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "delete_read_receipts_for_post_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrapf(err, "delete_read_receipts_for_post postId=%s", postID)
	}

	return nil
}

func (s *SqlPostReadReceiptStore) DeleteReadReceiptsForChannel(channelID string) error {
	query := s.getQueryBuilder().
		Delete("PostReadReceipts").
		Where(sq.Eq{"ChannelId": channelID})

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "delete_read_receipts_for_channel_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrapf(err, "delete_read_receipts_for_channel channelId=%s", channelID)
	}

	return nil
}

// Read receipt information and summaries

func (s *SqlPostReadReceiptStore) GetReadReceiptInfo(postID string) (*model.PostReadReceiptInfo, error) {
	// Get all receipts for the post
	receipts, err := s.GetReadReceiptsForPost(postID, false)
	if err != nil {
		return nil, err
	}

	// Get post info to determine channel members
	var channelId string
	postQuery := s.getQueryBuilder().
		Select("ChannelId").
		From("Posts").
		Where(sq.Eq{"Id": postID})

	queryString, args, queryErr := postQuery.ToSql()
	if queryErr != nil {
		return nil, errors.Wrap(queryErr, "get_read_receipt_info_post_tosql")
	}

	if err := s.GetReplica().Get(&channelId, queryString, args...); err != nil {
		return nil, errors.Wrapf(err, "get_read_receipt_info_post postId=%s", postID)
	}

	// Get total channel members count
	memberQuery := s.getQueryBuilder().
		Select("COUNT(*)").
		From("ChannelMembers").
		Where(sq.Eq{"ChannelId": channelId})

	memberQueryString, memberArgs, memberErr := memberQuery.ToSql()
	if memberErr != nil {
		return nil, errors.Wrap(memberErr, "get_read_receipt_info_members_tosql")
	}

	var totalUsers int
	if err := s.GetReplica().Get(&totalUsers, memberQueryString, memberArgs...); err != nil {
		return nil, errors.Wrapf(err, "get_read_receipt_info_members channelId=%s", channelId)
	}

	// Build receipt info
	info := &model.PostReadReceiptInfo{
		PostId:       postID,
		ChannelId:    channelId,
		ReadReceipts: receipts,
		TotalUsers:   totalUsers,
		ReadCount:    len(receipts),
	}

	// Set first and last read times
	if len(receipts) > 0 {
		info.LastRead = receipts[0].ReadAt  // receipts are ordered by ReadAt DESC
		info.FirstRead = receipts[len(receipts)-1].ReadAt
	}

	// Determine read status
	info.AllRead = info.ReadCount >= info.TotalUsers
	info.PartiallyRead = info.ReadCount > 0 && info.ReadCount < info.TotalUsers

	// Get unread users (this is a simplified version - in production you'd want to optimize this)
	if !info.AllRead {
		info.UnreadUsers = []string{} // Placeholder - would need more complex query
	}

	return info, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptInfoBatch(postIDs []string) (map[string]*model.PostReadReceiptInfo, error) {
	result := make(map[string]*model.PostReadReceiptInfo)

	for _, postID := range postIDs {
		info, err := s.GetReadReceiptInfo(postID)
		if err != nil {
			// Log error but continue processing other posts
			continue
		}
		result[postID] = info
	}

	return result, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptSummary(postID string) (*model.PostReadReceiptSummary, error) {
	query := s.getQueryBuilder().
		Select("PostId", "ChannelId", "ReadCount", "TotalRecipients", "LastUpdated", "FirstReadAt", "LastReadAt").
		From("PostReadReceiptSummary").
		Where(sq.Eq{"PostId": postID})

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_summary_tosql")
	}

	var summary model.PostReadReceiptSummary
	if err := s.GetReplica().Get(&summary, queryString, args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, store.NewErrNotFound("PostReadReceiptSummary", postID)
		}
		return nil, errors.Wrapf(err, "get_read_receipt_summary postId=%s", postID)
	}

	return &summary, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptSummariesForChannel(channelID string, since int64) ([]*model.PostReadReceiptSummary, error) {
	query := s.getQueryBuilder().
		Select("PostId", "ChannelId", "ReadCount", "TotalRecipients", "LastUpdated", "FirstReadAt", "LastReadAt").
		From("PostReadReceiptSummary").
		Where(sq.Eq{"ChannelId": channelID}).
		OrderBy("LastUpdated DESC")

	if since > 0 {
		query = query.Where(sq.GtOrEq{"LastUpdated": since})
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_summaries_for_channel_tosql")
	}

	var summaries []*model.PostReadReceiptSummary
	if err := s.GetReplica().Select(&summaries, queryString, args...); err != nil {
		return nil, errors.Wrapf(err, "get_read_receipt_summaries_for_channel channelId=%s", channelID)
	}

	return summaries, nil
}

func (s *SqlPostReadReceiptStore) UpdateReadReceiptSummary(summary *model.PostReadReceiptSummary) error {
	query := s.getQueryBuilder().
		Insert("PostReadReceiptSummary").
		Columns("PostId", "ChannelId", "ReadCount", "TotalRecipients", "LastUpdated", "FirstReadAt", "LastReadAt").
		Values(summary.PostId, summary.ChannelId, summary.ReadCount, summary.TotalRecipients, summary.LastUpdated, summary.FirstReadAt, summary.LastReadAt)

	// Handle upsert
	if s.DriverName() == model.DatabaseDriverPostgres {
		query = query.Suffix("ON CONFLICT (PostId) DO UPDATE SET ReadCount = ?, TotalRecipients = ?, LastUpdated = ?, FirstReadAt = ?, LastReadAt = ?",
			summary.ReadCount, summary.TotalRecipients, summary.LastUpdated, summary.FirstReadAt, summary.LastReadAt)
	} else {
		query = query.Suffix("ON DUPLICATE KEY UPDATE ReadCount = ?, TotalRecipients = ?, LastUpdated = ?, FirstReadAt = ?, LastReadAt = ?",
			summary.ReadCount, summary.TotalRecipients, summary.LastUpdated, summary.FirstReadAt, summary.LastReadAt)
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "update_read_receipt_summary_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrapf(err, "update_read_receipt_summary postId=%s", summary.PostId)
	}

	return nil
}

// Performance operations

func (s *SqlPostReadReceiptStore) CoalesceReadReceipts(channelID string, userID string, beforeTime int64) error {
	// This would implement batching/coalescing logic for performance
	// For now, it's a placeholder
	return nil
}

func (s *SqlPostReadReceiptStore) CleanupOldReadReceipts(daysOld int) (int64, error) {
	cutoff := model.GetMillis() - int64(daysOld*24*60*60*1000)

	query := s.getQueryBuilder().
		Delete("PostReadReceipts").
		Where(sq.Lt{"CreateAt": cutoff})

	queryString, args, err := query.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "cleanup_old_read_receipts_tosql")
	}

	result, err := s.GetMaster().Exec(queryString, args...)
	if err != nil {
		return 0, errors.Wrap(err, "cleanup_old_read_receipts")
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptStats(channelID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total receipts in channel
	countQuery := s.getQueryBuilder().
		Select("COUNT(*)").
		From("PostReadReceipts").
		Where(sq.Eq{"ChannelId": channelID})

	countQueryString, countArgs, err := countQuery.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_stats_count_tosql")
	}

	var totalReceipts int
	if err := s.GetReplica().Get(&totalReceipts, countQueryString, countArgs...); err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_stats_count")
	}

	stats["total_receipts"] = totalReceipts
	stats["channel_id"] = channelID
	stats["last_updated"] = model.GetMillis()

	return stats, nil
}

// Audit operations

func (s *SqlPostReadReceiptStore) SaveReadReceiptAuditLog(audit *model.ReadReceiptAuditLog) error {
	metadataJson, _ := json.Marshal(audit.Metadata)

	query := s.getQueryBuilder().
		Insert("ReadReceiptAuditLog").
		Columns("Id", "UserId", "PostId", "Action", "Metadata", "CreateAt").
		Values(audit.Id, audit.UserId, audit.PostId, audit.Action, metadataJson, audit.CreateAt)

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "save_read_receipt_audit_log_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrap(err, "save_read_receipt_audit_log")
	}

	return nil
}

func (s *SqlPostReadReceiptStore) GetReadReceiptAuditLogs(userID string, since int64, limit int) ([]*model.ReadReceiptAuditLog, error) {
	query := s.getQueryBuilder().
		Select("Id", "UserId", "PostId", "Action", "Metadata", "CreateAt").
		From("ReadReceiptAuditLog").
		Where(sq.Eq{"UserId": userID}).
		OrderBy("CreateAt DESC").
		Limit(uint64(limit))

	if since > 0 {
		query = query.Where(sq.GtOrEq{"CreateAt": since})
	}

	queryString, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_audit_logs_tosql")
	}

	rows, err := s.GetReplica().Query(queryString, args...)
	if err != nil {
		return nil, errors.Wrap(err, "get_read_receipt_audit_logs")
	}
	defer rows.Close()

	var logs []*model.ReadReceiptAuditLog
	for rows.Next() {
		var log model.ReadReceiptAuditLog
		var metadataJson []byte

		if err := rows.Scan(&log.Id, &log.UserId, &log.PostId, &log.Action, &metadataJson, &log.CreateAt); err != nil {
			return nil, errors.Wrap(err, "scan_read_receipt_audit_log")
		}

		if len(metadataJson) > 0 {
			json.Unmarshal(metadataJson, &log.Metadata)
		}

		logs = append(logs, &log)
	}

	return logs, nil
}

func (s *SqlPostReadReceiptStore) AnonymizeReadReceiptsForUser(userID string) error {
	// Replace user data with anonymized values for GDPR compliance
	query := s.getQueryBuilder().
		Update("PostReadReceipts").
		Set("DeviceId", "ANONYMIZED").
		Set("DeviceType", model.DeviceTypeUnknown).
		Set("SessionId", "").
		Where(sq.Eq{"UserId": userID})

	queryString, args, err := query.ToSql()
	if err != nil {
		return errors.Wrap(err, "anonymize_read_receipts_for_user_tosql")
	}

	if _, err := s.GetMaster().Exec(queryString, args...); err != nil {
		return errors.Wrapf(err, "anonymize_read_receipts_for_user userId=%s", userID)
	}

	return nil
}

// Ghost mode and utility methods

func (s *SqlPostReadReceiptStore) GetGhostReadReceipts(userID string, channelID string) ([]*model.PostReadReceipt, error) {
	// Ghost mode receipts would be stored in a separate table or marked differently
	// This is a placeholder implementation
	return []*model.PostReadReceipt{}, nil
}

func (s *SqlPostReadReceiptStore) SaveGhostReadReceipt(rctx request.CTX, receipt *model.PostReadReceipt) error {
	// Ghost mode implementation - would save to audit log only
	audit := &model.ReadReceiptAuditLog{
		Id:       model.NewId(),
		UserId:   receipt.UserId,
		PostId:   receipt.PostId,
		Action:   model.ReadReceiptActionGhostRead,
		Metadata: map[string]interface{}{
			"channel_id":  receipt.ChannelId,
			"device_type": receipt.DeviceType,
		},
		CreateAt: model.GetMillis(),
	}

	return s.SaveReadReceiptAuditLog(audit)
}

func (s *SqlPostReadReceiptStore) IsPostReadByUser(postID, userID string) (bool, error) {
	query := s.getQueryBuilder().
		Select("1").
		From("PostReadReceipts").
		Where(sq.And{
			sq.Eq{"PostId": postID},
			sq.Eq{"UserId": userID},
		})

	queryString, args, err := query.ToSql()
	if err != nil {
		return false, errors.Wrap(err, "is_post_read_by_user_tosql")
	}

	var result int
	err = s.GetReplica().Get(&result, queryString, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.Wrapf(err, "is_post_read_by_user postId=%s userId=%s", postID, userID)
	}

	return true, nil
}

func (s *SqlPostReadReceiptStore) GetUnreadPostsCount(channelID, userID string, since int64) (int64, error) {
	query := s.getQueryBuilder().
		Select("COUNT(DISTINCT p.Id)").
		From("Posts p").
		LeftJoin("PostReadReceipts prr ON p.Id = prr.PostId AND prr.UserId = ?", userID).
		Where(sq.And{
			sq.Eq{"p.ChannelId": channelID},
			sq.GtOrEq{"p.CreateAt": since},
			sq.Eq{"prr.PostId": nil}, // Not read
		})

	queryString, args, err := query.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "get_unread_posts_count_tosql")
	}

	var count int64
	if err := s.GetReplica().Get(&count, queryString, args...); err != nil {
		return 0, errors.Wrapf(err, "get_unread_posts_count channelId=%s userId=%s", channelID, userID)
	}

	return count, nil
}

func (s *SqlPostReadReceiptStore) GetLastReadTime(channelID, userID string) (int64, error) {
	query := s.getQueryBuilder().
		Select("MAX(ReadAt)").
		From("PostReadReceipts").
		Where(sq.And{
			sq.Eq{"ChannelId": channelID},
			sq.Eq{"UserId": userID},
		})

	queryString, args, err := query.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "get_last_read_time_tosql")
	}

	var lastRead sql.NullInt64
	if err := s.GetReplica().Get(&lastRead, queryString, args...); err != nil {
		return 0, errors.Wrapf(err, "get_last_read_time channelId=%s userId=%s", channelID, userID)
	}

	if lastRead.Valid {
		return lastRead.Int64, nil
	}

	return 0, nil
}