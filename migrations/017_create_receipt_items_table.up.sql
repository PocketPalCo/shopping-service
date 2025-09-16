-- Create receipt_items table with localization support
CREATE TABLE IF NOT EXISTS receipt_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id UUID NOT NULL REFERENCES users_receipts(id) ON DELETE CASCADE,
    item_order INTEGER NOT NULL,

    -- Original extracted data (immutable)
    original_description TEXT NOT NULL,
    original_language VARCHAR(5),

    -- User-facing localized data (mutable)
    localized_description TEXT,
    user_locale VARCHAR(5),

    -- Item details
    quantity DECIMAL(8,2) DEFAULT 1,
    unit_price DECIMAL(10,2),
    total_price DECIMAL(10,2) NOT NULL,
    currency_code VARCHAR(3) DEFAULT 'EUR',

    -- User customizations
    user_category VARCHAR(100),
    user_notes TEXT,
    is_user_modified BOOLEAN DEFAULT FALSE,

    -- AI extraction metadata
    confidence DECIMAL(5,3),
    bounding_regions JSONB,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_receipt_items_receipt_id ON receipt_items(receipt_id);
CREATE INDEX IF NOT EXISTS idx_receipt_items_original_desc ON receipt_items(original_description);
CREATE INDEX IF NOT EXISTS idx_receipt_items_localized_desc ON receipt_items(localized_description) WHERE localized_description IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receipt_items_category ON receipt_items(user_category) WHERE user_category IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receipt_items_order ON receipt_items(receipt_id, item_order);

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_receipt_items_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_receipt_items_updated_at
    BEFORE UPDATE ON receipt_items
    FOR EACH ROW
    EXECUTE FUNCTION update_receipt_items_updated_at();