-- Create translation cache table to avoid duplicate AI translation requests
CREATE TABLE translation_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_item TEXT NOT NULL,
    item_hash VARCHAR(64) NOT NULL, -- SHA256 hash of normalized original_item for fast lookups
    target_locale VARCHAR(10) NOT NULL,
    ai_response JSONB NOT NULL, -- Full AI response JSON for caching
    detected_language VARCHAR(10),
    confidence DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(item_hash, target_locale)
);

-- Index for fast cache lookups by hash and target locale
CREATE INDEX idx_translation_cache_lookup ON translation_cache(item_hash, target_locale);

-- Index for cleanup operations (older entries)
CREATE INDEX idx_translation_cache_created_at ON translation_cache(created_at);

-- Index for analytics and debugging by detected language
CREATE INDEX idx_translation_cache_detected_language ON translation_cache(detected_language);

COMMENT ON TABLE translation_cache IS 'Cache for AI translation responses to avoid duplicate requests';
COMMENT ON COLUMN translation_cache.original_item IS 'Original item name as received from receipt';
COMMENT ON COLUMN translation_cache.item_hash IS 'SHA256 hash of normalized original_item for fast lookups';
COMMENT ON COLUMN translation_cache.target_locale IS 'Target locale for translation (en, uk, ru, etc.)';
COMMENT ON COLUMN translation_cache.ai_response IS 'Complete AI translation response as JSONB';
COMMENT ON COLUMN translation_cache.detected_language IS 'Language detected by AI';
COMMENT ON COLUMN translation_cache.confidence IS 'AI confidence score for translation';