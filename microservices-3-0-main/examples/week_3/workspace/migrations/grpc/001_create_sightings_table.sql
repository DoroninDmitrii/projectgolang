-- +goose Up
CREATE TABLE sightings (
    uuid TEXT PRIMARY KEY,
    observed_at TIMESTAMPTZ,
    location TEXT,
    description TEXT,
    color TEXT,
    sound BOOLEAN,
    duration_seconds DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS sightings;
