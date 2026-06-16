-- +goose Up
INSERT INTO products (id, name, product_type, properties) VALUES
('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001', 'MacBook Pro 16"', 'laptop',
 '{"cpu": "Apple M3 Pro", "ram_gb": 36, "ssd_gb": 512}'),
('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002', 'iPhone 15 Pro', 'phone',
 '{"screen_size": 6.1, "battery_mah": 3274, "has_nfc": true}'),
('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003', 'Dell U2723QE', 'monitor',
 '{"resolution": "3840x2160", "panel_type": "IPS", "refresh_rate_hz": 60}');

-- +goose Down
DELETE FROM products WHERE id IN (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb001',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb002',
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbb003'
);
