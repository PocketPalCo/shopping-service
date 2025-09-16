-- Create item_translations dictionary table for common receipt items
CREATE TABLE IF NOT EXISTS item_translations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_text TEXT NOT NULL,
    original_language VARCHAR(5) NOT NULL,
    translated_text TEXT NOT NULL,
    target_language VARCHAR(5) NOT NULL,
    category VARCHAR(100),

    -- Translation metadata
    confidence DECIMAL(5,3),
    translation_source VARCHAR(50) DEFAULT 'user', -- 'user', 'ai', 'system'
    usage_count INTEGER DEFAULT 1,

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ensure unique combinations
    UNIQUE(original_text, original_language, target_language)
);

-- Add indexes for efficient lookups
CREATE INDEX IF NOT EXISTS idx_item_translations_original ON item_translations(original_text, original_language);
CREATE INDEX IF NOT EXISTS idx_item_translations_target ON item_translations(translated_text, target_language);
CREATE INDEX IF NOT EXISTS idx_item_translations_category ON item_translations(category) WHERE category IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_item_translations_usage ON item_translations(usage_count DESC);
CREATE INDEX IF NOT EXISTS idx_item_translations_source ON item_translations(translation_source);

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_item_translations_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_item_translations_updated_at
    BEFORE UPDATE ON item_translations
    FOR EACH ROW
    EXECUTE FUNCTION update_item_translations_updated_at();