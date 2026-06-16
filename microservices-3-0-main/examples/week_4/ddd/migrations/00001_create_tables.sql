-- +goose Up
-- создаём таблицу комплектующих с JSONB-свойствами и учётом резервирования
CREATE TABLE components (
    uuid           UUID PRIMARY KEY,
    name           VARCHAR(200) NOT NULL,
    type           VARCHAR(50)  NOT NULL,
    properties     JSONB        NOT NULL DEFAULT '{}',
    stock_quantity INT          NOT NULL DEFAULT 0,
    reserved       INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP
);

-- создаём таблицу сборок ПК
CREATE TABLE pc_builds (
    uuid       UUID PRIMARY KEY,
    status     VARCHAR(50) NOT NULL,
    created_at TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP
);

-- связующая таблица: комплектующие в сборке (нормализация many-to-many)
CREATE TABLE pc_build_components (
    uuid           UUID PRIMARY KEY,
    build_uuid     UUID NOT NULL REFERENCES pc_builds(uuid) ON DELETE CASCADE,
    component_uuid UUID NOT NULL REFERENCES components(uuid),
    quantity       INT  NOT NULL DEFAULT 1,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pc_build_components_build ON pc_build_components(build_uuid);
CREATE INDEX idx_pc_build_components_component ON pc_build_components(component_uuid);

-- +goose Down
-- удаляем таблицы в обратном порядке
DROP TABLE IF EXISTS pc_build_components;
DROP TABLE IF EXISTS pc_builds;
DROP TABLE IF EXISTS components;
