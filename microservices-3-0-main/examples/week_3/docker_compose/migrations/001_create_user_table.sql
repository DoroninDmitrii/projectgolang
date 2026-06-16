-- +goose Up
-- создаем таблицу пользователей
CREATE TABLE users (
    id         SERIAL PRIMARY KEY,
    username   TEXT,
    email      TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
-- удаляем таблицу пользователей
DROP TABLE IF EXISTS users;
