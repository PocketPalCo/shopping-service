-- Remove the incorrect unique constraint on original_item_id only
DROP INDEX IF EXISTS idx_item_mappings_unique_original;

-- Add correct unique constraint on the combination of original and parsed items
-- This allows one original item to map to multiple parsed items, but prevents duplicate mappings
CREATE UNIQUE INDEX idx_item_mappings_unique_pair ON item_mappings(original_item_id, parsed_item_id);