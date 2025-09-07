-- Create products table with standard product names and categories
CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en VARCHAR(255) NOT NULL,
    name_ru VARCHAR(255) NOT NULL,
    name_uk VARCHAR(255) NOT NULL,
    category VARCHAR(100) NOT NULL,
    subcategory VARCHAR(100) NOT NULL,
    aliases TEXT[], -- Array of possible aliases/variations for matching
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for fast lookups
CREATE INDEX idx_products_name_en ON products(name_en);
CREATE INDEX idx_products_name_ru ON products(name_ru);
CREATE INDEX idx_products_name_uk ON products(name_uk);
CREATE INDEX idx_products_category ON products(category);
CREATE INDEX idx_products_subcategory ON products(subcategory);
CREATE INDEX idx_products_aliases ON products USING GIN(aliases);

-- Insert some common products as examples
INSERT INTO products (name_en, name_ru, name_uk, category, subcategory, aliases) VALUES
-- Vegetables
('Carrots', 'морковка', 'морква', 'Vegetables', 'Root Vegetables', ARRAY['carrot', 'морковь', 'морква', 'морковочка']),
('Cabbage', 'капуста', 'капуста', 'Vegetables', 'Leafy Greens', ARRAY['cabbage', 'капуста']),
('Potatoes', 'картофель', 'картопля', 'Vegetables', 'Root Vegetables', ARRAY['potato', 'картошка', 'картоха', 'картофель']),
('Onions', 'лук', 'цибуля', 'Vegetables', 'Bulb Vegetables', ARRAY['onion', 'лук', 'цибуля', 'луковица']),
('Tomatoes', 'помидоры', 'помідори', 'Vegetables', 'Fruit Vegetables', ARRAY['tomato', 'помидор', 'помідор', 'томат', 'томаты']),

-- Meat & Poultry  
('Chicken Breast', 'куриное филе', 'куряче філе', 'Meat & Poultry', 'Chicken', ARRAY['chicken fillet', 'chicken breast', 'филе курицы', 'куриное филе', 'филе', 'курица филе', 'куряче філе', 'філе курки']),
('Chicken', 'курица', 'курка', 'Meat & Poultry', 'Chicken', ARRAY['chicken', 'курица', 'курка', 'цыпленок']),
('Beef', 'говядина', 'яловичина', 'Meat & Poultry', 'Beef', ARRAY['beef', 'говядина', 'яловичина']),
('Pork', 'свинина', 'свинина', 'Meat & Poultry', 'Pork', ARRAY['pork', 'свинина']),

-- Dairy
('Milk', 'молоко', 'молоко', 'Dairy', 'Milk', ARRAY['milk', 'молоко']),
('Cream', 'сливки', 'вершки', 'Dairy', 'Cream', ARRAY['cream', 'сливки', 'вершки', 'крем']),
('Cheese', 'сыр', 'сир', 'Dairy', 'Cheese', ARRAY['cheese', 'сыр', 'сир']),
('Butter', 'масло', 'масло', 'Dairy', 'Butter', ARRAY['butter', 'масло сливочное', 'вершкове масло']),

-- Bakery
('Bread', 'хлеб', 'хліб', 'Bakery', 'Bread', ARRAY['bread', 'хлеб', 'хліб', 'булка']),

-- Pantry
('Rice', 'рис', 'рис', 'Pantry', 'Grains', ARRAY['rice', 'рис']),
('Pasta', 'макароны', 'макарони', 'Pantry', 'Pasta', ARRAY['pasta', 'макароны', 'макарони', 'спагетти']),
('Salt', 'соль', 'сіль', 'Pantry', 'Seasonings', ARRAY['salt', 'соль', 'сіль']),
('Sugar', 'сахар', 'цукор', 'Pantry', 'Sweeteners', ARRAY['sugar', 'сахар', 'цукор']),

-- Pets
('Rabbit', 'кролик', 'кролик', 'Pets', 'Small Animals', ARRAY['rabbit', 'кролик', 'зайчик', 'bunny']);