-- Remove AI parsing fields from shopping_items table
ALTER TABLE shopping_items 
DROP COLUMN IF EXISTS original_item_id,
DROP COLUMN IF EXISTS parsed_item_id,
DROP COLUMN IF EXISTS display_name,
DROP COLUMN IF EXISTS parsed_name,
DROP COLUMN IF EXISTS parsing_status;