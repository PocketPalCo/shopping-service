CREATE TABLE IF NOT EXISTS family_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    added_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(family_id, user_id) -- Prevent duplicate memberships
);

-- Index for fast family member lookups
CREATE INDEX idx_family_members_family_id ON family_members(family_id);

-- Index for finding user's families
CREATE INDEX idx_family_members_user_id ON family_members(user_id);

-- Index for admin role queries
CREATE INDEX idx_family_members_role ON family_members(family_id, role) WHERE role = 'admin';