-- Remove locale field and index
DROP INDEX IF EXISTS idx_users_locale;
ALTER TABLE users DROP COLUMN locale;