-- Add AI parsing fields to shopping_items table
ALTER TABLE shopping_items 
ADD COLUMN original_item_id UUID REFERENCES original_items(id) ON DELETE SET NULL,
ADD COLUMN parsed_item_id UUID REFERENCES parsed_items(id) ON DELETE SET NULL,
ADD COLUMN display_name VARCHAR(255), -- Original user input for display
ADD COLUMN parsed_name VARCHAR(255), -- AI-parsed name for buttons
ADD COLUMN parsing_status VARCHAR(50) DEFAULT 'pending'; -- 'pending', 'parsed', 'failed'

-- Indexes for the new fields
CREATE INDEX idx_shopping_items_original_id ON shopping_items(original_item_id);
CREATE INDEX idx_shopping_items_parsed_id ON shopping_items(parsed_item_id);
CREATE INDEX idx_shopping_items_parsing_status ON shopping_items(parsing_status);