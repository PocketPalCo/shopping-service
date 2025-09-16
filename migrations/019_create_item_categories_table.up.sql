-- Create item_categories table for product categorization
CREATE TABLE IF NOT EXISTS item_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    name_en VARCHAR(100),
    name_uk VARCHAR(100),
    name_ru VARCHAR(100),

    -- Category metadata
    parent_category_id UUID REFERENCES item_categories(id) ON DELETE SET NULL,
    icon VARCHAR(50), -- emoji or icon name
    color VARCHAR(7), -- hex color code
    sort_order INTEGER DEFAULT 0,

    -- Statistics
    usage_count INTEGER DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(name)
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_item_categories_name ON item_categories(name);
CREATE INDEX IF NOT EXISTS idx_item_categories_parent ON item_categories(parent_category_id) WHERE parent_category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_item_categories_sort ON item_categories(sort_order);
CREATE INDEX IF NOT EXISTS idx_item_categories_usage ON item_categories(usage_count DESC);

-- Add trigger to update updated_at
CREATE OR REPLACE FUNCTION update_item_categories_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_item_categories_updated_at
    BEFORE UPDATE ON item_categories
    FOR EACH ROW
    EXECUTE FUNCTION update_item_categories_updated_at();

-- Insert default categories with multilingual support
INSERT INTO item_categories (name, name_en, name_uk, name_ru, icon, color, sort_order) VALUES
('Food & Beverages', 'Food & Beverages', 'Їжа та напої', 'Еда и напитки', '🍽️', '#FF6B6B', 1),
('Dairy Products', 'Dairy Products', 'Молочні продукти', 'Молочные продукты', '🥛', '#4ECDC4', 2),
('Meat & Fish', 'Meat & Fish', 'М''ясо та риба', 'Мясо и рыба', '🥩', '#45B7D1', 3),
('Fruits & Vegetables', 'Fruits & Vegetables', 'Фрукти та овочі', 'Фрукты и овощи', '🥕', '#96CEB4', 4),
('Bakery', 'Bakery', 'Хлібобулочні вироби', 'Хлебобулочные изделия', '🍞', '#FECA57', 5),
('Household Items', 'Household Items', 'Побутові товари', 'Бытовые товары', '🧽', '#A29BFE', 6),
('Personal Care', 'Personal Care', 'Особиста гігієна', 'Личная гигиена', '🧴', '#FD79A8', 7),
('Electronics', 'Electronics', 'Електроніка', 'Электроника', '📱', '#636E72', 8),
('Clothing', 'Clothing', 'Одяг', 'Одежда', '👕', '#E17055', 9),
('Other', 'Other', 'Інше', 'Другое', '📦', '#DDA0DD', 10);