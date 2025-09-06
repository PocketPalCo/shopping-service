-- Table to store AI-parsed standardized items (for training data)
CREATE TABLE IF NOT EXISTS parsed_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    standardized_name VARCHAR(255) NOT NULL,
    category VARCHAR(100), -- e.g., "dairy", "meat", "vegetables"
    subcategory VARCHAR(100), -- e.g., "milk", "chicken", "carrots"
    quantity_value DECIMAL(10,2), -- numeric value extracted
    quantity_unit VARCHAR(50), -- unit extracted (kg, liters, pieces, etc)
    language_code VARCHAR(5) NOT NULL, -- language this was parsed for
    confidence_score DECIMAL(3,2), -- AI confidence (0.00-1.00)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for efficient lookups and training queries
CREATE INDEX idx_parsed_items_standardized_name ON parsed_items(standardized_name);
CREATE INDEX idx_parsed_items_category ON parsed_items(category);
CREATE INDEX idx_parsed_items_language ON parsed_items(language_code);
CREATE INDEX idx_parsed_items_confidence ON parsed_items(confidence_score);
CREATE INDEX idx_parsed_items_created_at ON parsed_items(created_at);

-- Unique constraint to prevent duplicate standardized items per language
CREATE UNIQUE INDEX idx_parsed_items_unique_std_lang ON parsed_items(standardized_name, language_code);