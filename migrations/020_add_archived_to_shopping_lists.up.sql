-- Add is_archived column to shopping_lists table
ALTER TABLE shopping_lists ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT false;

-- Create index for fast lookups of non-archived lists
CREATE INDEX idx_shopping_lists_not_archived ON shopping_lists(is_archived) WHERE is_archived = false;