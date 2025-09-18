-- Fix corrupted translation cache entries by extracting individual translations
-- This migration fixes AI responses that contain batch translations instead of individual ones

-- Step 1: Update corrupted entries where ai_response contains multiple translations
-- Extract the matching translation for each item from the batch response
UPDATE translation_cache
SET ai_response = jsonb_build_object(
    'detected_language', ai_response->'detected_language',
    'target_language', ai_response->'target_language',
    'translations', jsonb_build_array(
        (
            SELECT t
            FROM jsonb_array_elements(ai_response->'translations') AS t
            WHERE LOWER(TRIM(t->>'original_text')) = LOWER(TRIM(original_item))
            LIMIT 1
        )
    ),
    'confidence', (
        SELECT (t->>'confidence')::decimal
        FROM jsonb_array_elements(ai_response->'translations') AS t
        WHERE LOWER(TRIM(t->>'original_text')) = LOWER(TRIM(original_item))
        LIMIT 1
    )
),
confidence = (
    SELECT (t->>'confidence')::decimal
    FROM jsonb_array_elements(ai_response->'translations') AS t
    WHERE LOWER(TRIM(t->>'original_text')) = LOWER(TRIM(original_item))
    LIMIT 1
)
WHERE jsonb_array_length(ai_response->'translations') > 1;

-- Step 2: Remove entries where no matching translation was found in the batch
-- These are truly corrupted and cannot be fixed
DELETE FROM translation_cache
WHERE NOT EXISTS (
    SELECT 1
    FROM jsonb_array_elements(ai_response->'translations') AS t
    WHERE LOWER(TRIM(t->>'original_text')) = LOWER(TRIM(original_item))
);

-- Add a comment to track when this fix was performed
COMMENT ON TABLE translation_cache IS 'Cache for AI translation responses to avoid duplicate requests. Fixed corrupted batch translation entries on 2024-09-16.';