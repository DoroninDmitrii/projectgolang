-- +goose Up
INSERT INTO accounts (uuid, owner, balance, description) VALUES
('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001', 'Алиса', 1000000, 'Основной счёт'),
('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002', 'Боб', 500000, NULL),
('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003', 'Карл', 250000, 'Сберегательный');

-- +goose Down
DELETE FROM accounts WHERE uuid IN (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa001',
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa002',
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaa003'
);
