CREATE TABLE IF NOT EXISTS models (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    runtime     TEXT NOT NULL,
    task        TEXT NOT NULL,
    model_path  TEXT NOT NULL,
    config      TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'stopped',
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_configs (
    runtime    TEXT PRIMARY KEY,
    config     TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS application_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
