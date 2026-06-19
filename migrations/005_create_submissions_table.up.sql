CREATE TABLE submissions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    language TEXT NOT NULL,
    source TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    verdict TEXT DEFAULT NULL,
    runtime_ms INTEGER DEFAULT NULL,
    memory_kb INTEGER DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT submissions_status_check
        CHECK (status IN ('queued', 'running', 'done', 'failed')),

    CONSTRAINT submissions_verdict_check
        CHECK (verdict IS NULL OR verdict IN ('AC', 'WA', 'TLE', 'MLE', 'RE', 'CE'))
);
