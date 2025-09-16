-- Drop trigger and function for item_translations
DROP TRIGGER IF EXISTS trigger_update_item_translations_updated_at ON item_translations;
DROP FUNCTION IF EXISTS update_item_translations_updated_at();

-- Drop indexes
DROP INDEX IF EXISTS idx_item_translations_source;
DROP INDEX IF EXISTS idx_item_translations_usage;
DROP INDEX IF EXISTS idx_item_translations_category;
DROP INDEX IF EXISTS idx_item_translations_target;
DROP INDEX IF EXISTS idx_item_translations_original;

-- Drop item_translations table
DROP TABLE IF EXISTS item_translations;