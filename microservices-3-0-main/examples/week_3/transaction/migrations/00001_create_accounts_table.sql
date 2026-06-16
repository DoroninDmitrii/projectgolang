-- +goose Up
CREATE TABLE accounts (
    uuid UUID PRIMARY KEY,
    owner VARCHAR(100) NOT NULL,
    balance BIGINT NOT NULL DEFAULT 0,
    description TEXT,                    -- nullable → *string в Go
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP                 -- nullable → *time.Time в Go
);

-- +goose Down
DROP TABLE IF EXISTS accounts;
