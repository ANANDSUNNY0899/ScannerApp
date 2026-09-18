CREATE TABLE IF NOT EXISTS receipt_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vendor_name VARCHAR(255),
    category VARCHAR(100),       -- e.g., 'Construction', 'Dining', 'Hardware'
    description TEXT,            -- e.g., '100 Bags of Cement', 'Office Supplies'
    total_price NUMERIC(10, 2),
    order_date DATE,
    image_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_receipt_orders_user_date ON receipt_orders(user_id, order_date);
