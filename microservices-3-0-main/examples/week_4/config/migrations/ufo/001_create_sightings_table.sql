-- +goose Up
CREATE TABLE sightings (
    uuid            TEXT PRIMARY KEY,
    observed_at     TIMESTAMPTZ,
    location        TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    color           TEXT,
    sound           BOOLEAN,
    duration_seconds INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS sightings;
