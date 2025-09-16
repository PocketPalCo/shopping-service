-- Remove index and column for is_archived
DROP INDEX IF EXISTS idx_shopping_lists_not_archived;
ALTER TABLE shopping_lists DROP COLUMN IF EXISTS is_archived;