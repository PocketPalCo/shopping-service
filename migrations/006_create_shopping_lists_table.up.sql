CREATE TABLE IF NOT EXISTS shopping_lists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id UUID REFERENCES families(id) ON DELETE CASCADE,
    is_shared BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for fast lookups by owner
CREATE INDEX idx_shopping_lists_owner_id ON shopping_lists(owner_id);

-- Index for family-based lists
CREATE INDEX idx_shopping_lists_family_id ON shopping_lists(family_id);

-- Index for shared lists
CREATE INDEX idx_shopping_lists_shared ON shopping_lists(is_shared) WHERE is_shared = true;

-- Index for name-based searches
CREATE INDEX idx_shopping_lists_name ON shopping_lists(name);