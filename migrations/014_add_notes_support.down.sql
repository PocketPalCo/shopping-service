-- Remove notes support from parsed_items table
DROP INDEX IF EXISTS idx_parsed_items_notes;
ALTER TABLE parsed_items DROP COLUMN IF EXISTS notes;

-- Remove notes support from shopping_items table
DROP INDEX IF EXISTS idx_shopping_items_notes;
ALTER TABLE shopping_items DROP COLUMN IF EXISTS notes;