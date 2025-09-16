-- Drop trigger and function for item_categories
DROP TRIGGER IF EXISTS trigger_update_item_categories_updated_at ON item_categories;
DROP FUNCTION IF EXISTS update_item_categories_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_item_categories_usage;
DROP INDEX IF EXISTS idx_item_categories_sort;
DROP INDEX IF EXISTS idx_item_categories_parent;
DROP INDEX IF EXISTS idx_item_categories_name;

-- Drop item_categories table
DROP TABLE IF EXISTS item_categories;