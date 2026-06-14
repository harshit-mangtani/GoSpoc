CREATE TABLE problems (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    statement TEXT NOT NULL,
    time_limit_ms INTEGER NOT NULL,
    memory_limit_kb INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
