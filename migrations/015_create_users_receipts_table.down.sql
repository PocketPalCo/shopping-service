-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_update_users_receipts_updated_at ON users_receipts;
DROP FUNCTION IF EXISTS update_users_receipts_updated_at();

-- Drop indexes (if they exist)
DROP INDEX IF EXISTS idx_users_receipts_user_id;
DROP INDEX IF EXISTS idx_users_receipts_created_at;
DROP INDEX IF EXISTS idx_users_receipts_processed;
DROP INDEX IF EXISTS idx_users_receipts_merchant_name;

-- Drop table
DROP TABLE IF EXISTS users_receipts;