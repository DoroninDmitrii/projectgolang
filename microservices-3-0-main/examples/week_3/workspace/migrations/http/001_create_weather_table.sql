-- +goose Up
CREATE TABLE weather (
    city TEXT PRIMARY KEY,
    temperature DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS weather;
