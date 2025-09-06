-- Add locale field to users table
ALTER TABLE users ADD COLUMN locale VARCHAR(5) NOT NULL DEFAULT 'en';

-- Create index for locale lookups
CREATE INDEX idx_users_locale ON users(locale);

-- Update existing users to use 'en' as default
UPDATE users SET locale = 'en' WHERE locale IS NULL;