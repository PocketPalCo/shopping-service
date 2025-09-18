-- Rollback migration for corrupted translation cache fix
-- This migration cannot be fully rolled back since we modified the ai_response data
-- However, we can restore the original table comment

COMMENT ON TABLE translation_cache IS 'Cache for AI translation responses to avoid duplicate requests';