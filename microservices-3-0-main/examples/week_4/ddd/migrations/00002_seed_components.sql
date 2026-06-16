-- +goose Up
INSERT INTO components (uuid, name, type, properties, stock_quantity) VALUES
    -- Материнские платы
    ('cccccccc-0000-0000-0000-000000000001', 'ASUS ROG Strix Z790', 'motherboard',
     '{"motherboard": {"socket": "LGA1700", "ram_type": "DDR5", "ram_slots": 4}}', 3),
    ('cccccccc-0000-0000-0000-000000000002', 'MSI MAG B650', 'motherboard',
     '{"motherboard": {"socket": "AM5", "ram_type": "DDR5", "ram_slots": 2}}', 2),

    -- Процессоры
    ('cccccccc-0000-0000-0000-000000000011', 'Intel Core i7-13700K', 'cpu',
     '{"cpu": {"socket": "LGA1700", "cores": 16, "tdp_watts": 125}}', 5),
    ('cccccccc-0000-0000-0000-000000000012', 'AMD Ryzen 9 7950X', 'cpu',
     '{"cpu": {"socket": "AM5", "cores": 16, "tdp_watts": 170}}', 3),

    -- Оперативная память
    ('cccccccc-0000-0000-0000-000000000021', 'Kingston Fury Beast DDR5', 'ram',
     '{"ram": {"ram_type": "DDR5", "capacity_gb": 32}}', 10),
    ('cccccccc-0000-0000-0000-000000000022', 'Corsair Vengeance DDR4', 'ram',
     '{"ram": {"ram_type": "DDR4", "capacity_gb": 16}}', 8),

    -- Видеокарты
    ('cccccccc-0000-0000-0000-000000000031', 'NVIDIA RTX 4070', 'gpu',
     '{"gpu": {"required_tdp_watts": 200, "vram_gb": 12}}', 4),
    ('cccccccc-0000-0000-0000-000000000032', 'NVIDIA RTX 4090', 'gpu',
     '{"gpu": {"required_tdp_watts": 450, "vram_gb": 24}}', 1);

-- +goose Down
DELETE FROM components WHERE uuid IN (
    'cccccccc-0000-0000-0000-000000000001',
    'cccccccc-0000-0000-0000-000000000002',
    'cccccccc-0000-0000-0000-000000000011',
    'cccccccc-0000-0000-0000-000000000012',
    'cccccccc-0000-0000-0000-000000000021',
    'cccccccc-0000-0000-0000-000000000022',
    'cccccccc-0000-0000-0000-000000000031',
    'cccccccc-0000-0000-0000-000000000032'
);
