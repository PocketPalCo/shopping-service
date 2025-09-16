-- Remove indexes
DROP INDEX IF EXISTS idx_users_receipts_detected_language;
DROP INDEX IF EXISTS idx_users_receipts_transaction_date_time;
DROP INDEX IF EXISTS idx_users_receipts_receipt_type;
DROP INDEX IF EXISTS idx_users_receipts_country_region;

-- Remove added columns from users_receipts table
ALTER TABLE users_receipts DROP COLUMN IF EXISTS last_translation_update;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS auto_translation_enabled;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS content_locale;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS detected_language;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS raw_ai_response;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS extraction_model_version;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS ai_confidence;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS net_amount;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS total_tax;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS currency_code;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS receipt_type;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS transaction_time;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS country_region;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS merchant_phone;
ALTER TABLE users_receipts DROP COLUMN IF EXISTS merchant_address;