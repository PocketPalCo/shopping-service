CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token VARCHAR(255) UNIQUE NOT NULL,
    chat_id BIGINT NOT NULL,
    last_activity TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT (NOW() + INTERVAL '7 days'),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for fast session lookups
CREATE INDEX idx_user_sessions_token ON user_sessions(session_token);

-- Index for cleanup of expired sessions
CREATE INDEX idx_user_sessions_expires ON user_sessions(expires_at);

-- Index for user sessions
CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);