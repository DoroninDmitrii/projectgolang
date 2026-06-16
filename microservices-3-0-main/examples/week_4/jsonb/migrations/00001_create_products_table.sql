-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    product_type VARCHAR(50) NOT NULL,         -- 'laptop', 'phone', 'monitor'
    properties JSONB NOT NULL DEFAULT '{}',     -- типоспецифичные свойства
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP                        -- nullable → *time.Time в Go
);

-- +goose Down
DROP TABLE IF EXISTS products;
