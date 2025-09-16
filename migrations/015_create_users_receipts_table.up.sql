CREATE TABLE IF NOT EXISTS users_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_url TEXT NOT NULL, -- URL to the stored receipt image in cloud storage
    file_name TEXT NOT NULL, -- Original filename
    file_size BIGINT NOT NULL, -- File size in bytes
    content_type TEXT NOT NULL, -- MIME type (e.g., 'image/jpeg')
    telegram_file_id TEXT, -- Telegram file ID for reference
    processed BOOLEAN DEFAULT FALSE, -- Whether receipt has been processed by AI
    processing_error TEXT, -- Error message if processing failed
    merchant_name TEXT, -- Extracted merchant name (after processing)
    total_amount DECIMAL(10,2), -- Extracted total amount
    transaction_date DATE, -- Extracted transaction date
    items_count INTEGER DEFAULT 0, -- Number of items extracted
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add indexes for common queries
CREATE INDEX IF NOT EXISTS idx_users_receipts_user_id ON users_receipts(user_id);
CREATE INDEX IF NOT EXISTS idx_users_receipts_created_at ON users_receipts(created_at);
CREATE INDEX IF NOT EXISTS idx_users_receipts_processed ON users_receipts(processed);
CREATE INDEX IF NOT EXISTS idx_users_receipts_merchant_name ON users_receipts(merchant_name) WHERE merchant_name IS NOT NULL;

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_users_receipts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_users_receipts_updated_at
    BEFORE UPDATE ON users_receipts
    FOR EACH ROW
    EXECUTE FUNCTION update_users_receipts_updated_at();