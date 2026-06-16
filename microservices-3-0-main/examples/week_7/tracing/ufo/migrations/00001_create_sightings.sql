-- +goose Up
-- Создаём таблицу наблюдений НЛО
CREATE TABLE sightings (
    uuid             TEXT PRIMARY KEY,
    observed_at      TIMESTAMPTZ,
    location         TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    color            TEXT,
    sound            BOOLEAN,
    duration_seconds INTEGER,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ,
    deleted_at       TIMESTAMPTZ
);

-- +goose Down
-- Удаляем таблицу наблюдений НЛО
DROP TABLE IF EXISTS sightings;
