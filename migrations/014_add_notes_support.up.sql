-- Add notes support to parsed_items table
ALTER TABLE parsed_items ADD COLUMN notes TEXT;

-- Add notes support to shopping_items table
ALTER TABLE shopping_items ADD COLUMN notes TEXT;

-- Add index for notes searching if needed in the future
CREATE INDEX idx_parsed_items_notes ON parsed_items(notes) WHERE notes IS NOT NULL;
CREATE INDEX idx_shopping_items_notes ON shopping_items(notes) WHERE notes IS NOT NULL;