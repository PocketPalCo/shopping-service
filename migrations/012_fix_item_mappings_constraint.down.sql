-- Rollback: Remove the new constraint and restore the old one
DROP INDEX IF EXISTS idx_item_mappings_unique_pair;

-- Restore the original constraint (though this was problematic)
CREATE UNIQUE INDEX idx_item_mappings_unique_original ON item_mappings(original_item_id);