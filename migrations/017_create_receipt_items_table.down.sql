-- Drop trigger and function for receipt_items
DROP TRIGGER IF EXISTS trigger_update_receipt_items_updated_at ON receipt_items;
DROP FUNCTION IF EXISTS update_receipt_items_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_receipt_items_order;
DROP INDEX IF EXISTS idx_receipt_items_category;
DROP INDEX IF EXISTS idx_receipt_items_localized_desc;
DROP INDEX IF EXISTS idx_receipt_items_original_desc;
DROP INDEX IF EXISTS idx_receipt_items_receipt_id;

-- Drop receipt_items table
DROP TABLE IF EXISTS receipt_items;