// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
	"net/http"
)

// PostReadReceipt represents a single read receipt for a post
type PostReadReceipt struct {
	PostId     string `json:"post_id" db:"PostId"`
	UserId     string `json:"user_id" db:"UserId"`
	ChannelId  string `json:"channel_id" db:"ChannelId"`
	ReadAt     int64  `json:"read_at" db:"ReadAt"`
	CreateAt   int64  `json:"create_at" db:"CreateAt"`
	DeviceId   string `json:"device_id,omitempty" db:"DeviceId"`
	DeviceType string `json:"device_type,omitempty" db:"DeviceType"`
	SessionId  string `json:"session_id,omitempty" db:"SessionId"`
}

// PostReadReceiptInfo contains comprehensive read receipt information for a post
type PostReadReceiptInfo struct {
	PostId          string             `json:"post_id"`
	ChannelId       string             `json:"channel_id"`
	ReadReceipts    []*PostReadReceipt `json:"read_receipts"`
	UnreadUsers     []string           `json:"unread_users"`
	TotalUsers      int                `json:"total_users"`
	ReadCount       int                `json:"read_count"`
	LastRead        int64              `json:"last_read,omitempty"`
	FirstRead       int64              `json:"first_read,omitempty"`
	PartiallyRead   bool               `json:"partially_read"`
	AllRead         bool               `json:"all_read"`
}

// PostReadReceiptSummary optimized summary for quick lookups
type PostReadReceiptSummary struct {
	PostId          string `json:"post_id" db:"PostId"`
	ChannelId       string `json:"channel_id" db:"ChannelId"`
	ReadCount       int    `json:"read_count" db:"ReadCount"`
	TotalRecipients int    `json:"total_recipients" db:"TotalRecipients"`
	LastUpdated     int64  `json:"last_updated" db:"LastUpdated"`
	FirstReadAt     int64  `json:"first_read_at,omitempty" db:"FirstReadAt"`
	LastReadAt      int64  `json:"last_read_at,omitempty" db:"LastReadAt"`
}

// ReadReceiptAuditLog for tracking privacy-sensitive operations
type ReadReceiptAuditLog struct {
	Id       string                 `json:"id" db:"Id"`
	UserId   string                 `json:"user_id" db:"UserId"`
	PostId   string                 `json:"post_id" db:"PostId"`
	Action   string                 `json:"action" db:"Action"`
	Metadata map[string]interface{} `json:"metadata,omitempty" db:"Metadata"`
	CreateAt int64                  `json:"create_at" db:"CreateAt"`
}

// PostReadReceiptBatch for batch operations
type PostReadReceiptBatch struct {
	PostIds   []string `json:"post_ids"`
	UserId    string   `json:"user_id"`
	ChannelId string   `json:"channel_id"`
	ReadAt    int64    `json:"read_at"`
	DeviceId  string   `json:"device_id,omitempty"`
}

// ReadReceiptRequest represents a request to mark posts as read
type ReadReceiptRequest struct {
	PostId    string `json:"post_id"`
	ReadAt    int64  `json:"read_at,omitempty"`
	DeviceId  string `json:"device_id,omitempty"`
}

// ReadReceiptBatchRequest for batch read operations
type ReadReceiptBatchRequest struct {
	PostIds   []string `json:"post_ids"`
	ChannelId string   `json:"channel_id"`
	ReadAt    int64    `json:"read_at,omitempty"`
	DeviceId  string   `json:"device_id,omitempty"`
}

// Constants for read receipt system
const (
	ReadReceiptActionRead        = "read"
	ReadReceiptActionGhostRead   = "ghost_read"
	ReadReceiptActionBulkRead    = "bulk_read"
	ReadReceiptActionPrivacyView = "privacy_view"
	
	DeviceTypeDesktop = "desktop"
	DeviceTypeMobile  = "mobile"
	DeviceTypeWeb     = "web"
	DeviceTypeUnknown = "unknown"
	
	ReadReceiptMaxBatchSize = 100
	ReadReceiptCleanupDays  = 30
)

// IsValid validates the PostReadReceipt
func (r *PostReadReceipt) IsValid() *AppError {
	if !IsValidId(r.PostId) {
		return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.post_id.app_error", nil, "", http.StatusBadRequest)
	}

	if !IsValidId(r.UserId) {
		return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.user_id.app_error", nil, "", http.StatusBadRequest)
	}

	if !IsValidId(r.ChannelId) {
		return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.channel_id.app_error", nil, "", http.StatusBadRequest)
	}

	if r.ReadAt == 0 {
		return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.read_at.app_error", nil, "", http.StatusBadRequest)
	}

	if r.CreateAt == 0 {
		return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.create_at.app_error", nil, "", http.StatusBadRequest)
	}

	// Validate device type if provided
	if r.DeviceType != "" {
		validDeviceTypes := []string{DeviceTypeDesktop, DeviceTypeMobile, DeviceTypeWeb, DeviceTypeUnknown}
		isValid := false
		for _, validType := range validDeviceTypes {
			if r.DeviceType == validType {
				isValid = true
				break
			}
		}
		if !isValid {
			return NewAppError("PostReadReceipt.IsValid", "model.post_read_receipt.is_valid.device_type.app_error", nil, "", http.StatusBadRequest)
		}
	}

	return nil
}

// PreSave prepares the read receipt for saving
func (r *PostReadReceipt) PreSave() {
	if r.CreateAt == 0 {
		r.CreateAt = GetMillis()
	}

	if r.ReadAt == 0 {
		r.ReadAt = GetMillis()
	}

	if r.DeviceType == "" {
		r.DeviceType = DeviceTypeUnknown
	}
}

// ToJSON converts PostReadReceipt to JSON string
func (r *PostReadReceipt) ToJSON() (string, error) {
	b, err := json.Marshal(r)
	return string(b), err
}

// PostReadReceiptFromJSON creates PostReadReceipt from JSON string
func PostReadReceiptFromJSON(data string) (*PostReadReceipt, error) {
	var r PostReadReceipt
	err := json.Unmarshal([]byte(data), &r)
	return &r, err
}

// IsValid validates the ReadReceiptRequest
func (r *ReadReceiptRequest) IsValid() *AppError {
	if !IsValidId(r.PostId) {
		return NewAppError("ReadReceiptRequest.IsValid", "model.read_receipt_request.is_valid.post_id.app_error", nil, "", http.StatusBadRequest)
	}

	return nil
}

// IsValid validates the ReadReceiptBatchRequest
func (r *ReadReceiptBatchRequest) IsValid() *AppError {
	if len(r.PostIds) == 0 {
		return NewAppError("ReadReceiptBatchRequest.IsValid", "model.read_receipt_batch_request.is_valid.post_ids.app_error", nil, "", http.StatusBadRequest)
	}

	if len(r.PostIds) > ReadReceiptMaxBatchSize {
		return NewAppError("ReadReceiptBatchRequest.IsValid", "model.read_receipt_batch_request.is_valid.batch_size.app_error", nil, "", http.StatusBadRequest)
	}

	for _, postId := range r.PostIds {
		if !IsValidId(postId) {
			return NewAppError("ReadReceiptBatchRequest.IsValid", "model.read_receipt_batch_request.is_valid.post_id.app_error", nil, "", http.StatusBadRequest)
		}
	}

	if r.ChannelId != "" && !IsValidId(r.ChannelId) {
		return NewAppError("ReadReceiptBatchRequest.IsValid", "model.read_receipt_batch_request.is_valid.channel_id.app_error", nil, "", http.StatusBadRequest)
	}

	return nil
}

// Auditable returns auditable fields for PostReadReceipt
func (r *PostReadReceipt) Auditable() map[string]any {
	return map[string]any{
		"post_id":     r.PostId,
		"user_id":     r.UserId,
		"channel_id":  r.ChannelId,
		"read_at":     r.ReadAt,
		"create_at":   r.CreateAt,
		"device_type": r.DeviceType,
	}
}

// IsFullyRead returns true if all users have read the post
func (info *PostReadReceiptInfo) IsFullyRead() bool {
	return info.ReadCount >= info.TotalUsers
}

// IsPartiallyRead returns true if some but not all users have read the post
func (info *PostReadReceiptInfo) IsPartiallyRead() bool {
	return info.ReadCount > 0 && info.ReadCount < info.TotalUsers
}

// GetReadPercentage returns the percentage of users who have read the post
func (info *PostReadReceiptInfo) GetReadPercentage() float64 {
	if info.TotalUsers == 0 {
		return 0
	}
	return float64(info.ReadCount) / float64(info.TotalUsers) * 100
}

// ToJSON converts PostReadReceiptInfo to JSON string
func (info *PostReadReceiptInfo) ToJSON() (string, error) {
	b, err := json.Marshal(info)
	return string(b), err
}

// Equals compares two PostReadReceipts
func (r *PostReadReceipt) Equals(other *PostReadReceipt) bool {
	if other == nil {
		return false
	}
	return r.PostId == other.PostId && 
		   r.UserId == other.UserId && 
		   r.ReadAt == other.ReadAt
}

// Clone creates a deep copy of PostReadReceipt
func (r *PostReadReceipt) Clone() *PostReadReceipt {
	return &PostReadReceipt{
		PostId:     r.PostId,
		UserId:     r.UserId,
		ChannelId:  r.ChannelId,
		ReadAt:     r.ReadAt,
		CreateAt:   r.CreateAt,
		DeviceId:   r.DeviceId,
		DeviceType: r.DeviceType,
		SessionId:  r.SessionId,
	}
}