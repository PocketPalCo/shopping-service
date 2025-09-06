CREATE TABLE IF NOT EXISTS shopping_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    list_id UUID NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    quantity TEXT,
    is_completed BOOLEAN NOT NULL DEFAULT false,
    added_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    completed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for fast lookups by list
CREATE INDEX idx_shopping_items_list_id ON shopping_items(list_id);

-- Index for completion status
CREATE INDEX idx_shopping_items_completed ON shopping_items(list_id, is_completed);

-- Index for user's added items
CREATE INDEX idx_shopping_items_added_by ON shopping_items(added_by);