-- Table to store original user input items (for training data)
CREATE TABLE IF NOT EXISTS original_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    raw_text VARCHAR(255) NOT NULL,
    language_code VARCHAR(5) NOT NULL, -- en, uk, ru
    quantity_raw TEXT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for efficient lookups
CREATE INDEX idx_original_items_raw_text ON original_items(raw_text);
CREATE INDEX idx_original_items_language ON original_items(language_code);
CREATE INDEX idx_original_items_user_id ON original_items(user_id);
CREATE INDEX idx_original_items_created_at ON original_items(created_at);