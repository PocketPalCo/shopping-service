-- Enhance users_receipts table with additional fields from AI analysis
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS merchant_address TEXT;
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS merchant_phone TEXT;
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS country_region VARCHAR(3);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS transaction_time TIME;
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS receipt_type VARCHAR(50);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS currency_code VARCHAR(3) DEFAULT 'EUR';
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS total_tax DECIMAL(10,2);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS net_amount DECIMAL(10,2);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS ai_confidence DECIMAL(5,3);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS extraction_model_version VARCHAR(50);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS raw_ai_response JSONB;
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS detected_language VARCHAR(5);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS content_locale VARCHAR(5);
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS auto_translation_enabled BOOLEAN DEFAULT TRUE;
ALTER TABLE users_receipts ADD COLUMN IF NOT EXISTS last_translation_update TIMESTAMP WITH TIME ZONE;

-- Add indexes for new fields
CREATE INDEX IF NOT EXISTS idx_users_receipts_country_region ON users_receipts(country_region) WHERE country_region IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_receipts_receipt_type ON users_receipts(receipt_type) WHERE receipt_type IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_receipts_transaction_date_time ON users_receipts(transaction_date, transaction_time) WHERE transaction_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_receipts_detected_language ON users_receipts(detected_language) WHERE detected_language IS NOT NULL;