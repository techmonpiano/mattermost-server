-- Rollback migration: Archive tables instead of dropping to preserve data
-- This follows the pattern of preserving data during rollbacks

-- Get current timestamp for archive naming
DO $$
DECLARE
    archive_suffix TEXT;
BEGIN
    archive_suffix := 'archived_' || EXTRACT(EPOCH FROM NOW())::bigint::text;
    
    -- Archive tables with timestamp suffix
    EXECUTE format('ALTER TABLE IF EXISTS PostReadReceipts RENAME TO PostReadReceipts_%s', archive_suffix);
    EXECUTE format('ALTER TABLE IF EXISTS PostReadReceiptSummary RENAME TO PostReadReceiptSummary_%s', archive_suffix);  
    EXECUTE format('ALTER TABLE IF EXISTS ReadReceiptAuditLog RENAME TO ReadReceiptAuditLog_%s', archive_suffix);
END
$$;

-- Note: Indexes will be automatically renamed with the table
-- This approach preserves all data for potential recovery while effectively removing the feature