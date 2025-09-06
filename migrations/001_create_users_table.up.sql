CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT UNIQUE NOT NULL,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    is_authorized BOOLEAN NOT NULL DEFAULT false,
    authorized_by UUID REFERENCES users(id) ON DELETE SET NULL,
    authorized_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for fast telegram_id lookups
CREATE INDEX idx_users_telegram_id ON users(telegram_id);

-- Index for authorized users
CREATE INDEX idx_users_authorized ON users(is_authorized) WHERE is_authorized = true;