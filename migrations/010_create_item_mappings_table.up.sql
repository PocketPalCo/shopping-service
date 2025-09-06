-- Table to map original items to parsed items (for training data)
CREATE TABLE IF NOT EXISTS item_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_item_id UUID NOT NULL REFERENCES original_items(id) ON DELETE CASCADE,
    parsed_item_id UUID NOT NULL REFERENCES parsed_items(id) ON DELETE CASCADE,
    mapping_method VARCHAR(50) NOT NULL, -- 'ai_openai', 'ai_llama', 'manual', 'user_correction'
    is_validated BOOLEAN DEFAULT false, -- human validation status
    validated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    validated_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for efficient lookups
CREATE INDEX idx_item_mappings_original_id ON item_mappings(original_item_id);
CREATE INDEX idx_item_mappings_parsed_id ON item_mappings(parsed_item_id);
CREATE INDEX idx_item_mappings_method ON item_mappings(mapping_method);
CREATE INDEX idx_item_mappings_validated ON item_mappings(is_validated);
CREATE INDEX idx_item_mappings_created_at ON item_mappings(created_at);

-- Unique constraint to prevent duplicate mappings
CREATE UNIQUE INDEX idx_item_mappings_unique_original ON item_mappings(original_item_id);