-- Create PostReadReceipts table for tracking read receipts in DMs and GMs
CREATE TABLE IF NOT EXISTS PostReadReceipts (
    PostId      varchar(26) NOT NULL,
    UserId      varchar(26) NOT NULL,
    ChannelId   varchar(26) NOT NULL,
    ReadAt      bigint NOT NULL,
    CreateAt    bigint NOT NULL,
    DeviceId    varchar(128),
    DeviceType  varchar(20),
    SessionId   varchar(26),
    PRIMARY KEY (PostId, UserId)
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_post_read_receipts_post_id ON PostReadReceipts (PostId);
CREATE INDEX IF NOT EXISTS idx_post_read_receipts_user_id ON PostReadReceipts (UserId);
CREATE INDEX IF NOT EXISTS idx_post_read_receipts_channel_id ON PostReadReceipts (ChannelId);
CREATE INDEX IF NOT EXISTS idx_post_read_receipts_read_at ON PostReadReceipts (ReadAt);
CREATE INDEX IF NOT EXISTS idx_post_read_receipts_user_post_read ON PostReadReceipts (UserId, PostId, ReadAt DESC);

-- Create summary table for performance optimization
CREATE TABLE IF NOT EXISTS PostReadReceiptSummary (
    PostId varchar(26) PRIMARY KEY,
    ChannelId varchar(26) NOT NULL,
    ReadCount int NOT NULL DEFAULT 0,
    TotalRecipients int NOT NULL,
    LastUpdated bigint NOT NULL,
    FirstReadAt bigint,
    LastReadAt bigint
);

-- Create indexes for summary table
CREATE INDEX IF NOT EXISTS idx_summary_channel ON PostReadReceiptSummary (ChannelId);
CREATE INDEX IF NOT EXISTS idx_summary_last_updated ON PostReadReceiptSummary (LastUpdated);
CREATE INDEX IF NOT EXISTS idx_summary_read_ratio ON PostReadReceiptSummary (ReadCount, TotalRecipients);

-- Create audit log table for ghost mode and privacy compliance
CREATE TABLE IF NOT EXISTS ReadReceiptAuditLog (
    Id varchar(26) PRIMARY KEY,
    UserId varchar(26) NOT NULL,
    PostId varchar(26) NOT NULL,
    Action varchar(50) NOT NULL,
    Metadata TEXT,
    CreateAt bigint NOT NULL
);

-- Create indexes for audit log
CREATE INDEX IF NOT EXISTS idx_audit_user ON ReadReceiptAuditLog (UserId);
CREATE INDEX IF NOT EXISTS idx_audit_post ON ReadReceiptAuditLog (PostId);
CREATE INDEX IF NOT EXISTS idx_audit_create ON ReadReceiptAuditLog (CreateAt);
CREATE INDEX IF NOT EXISTS idx_audit_action ON ReadReceiptAuditLog (Action);