CREATE TABLE submission_test_results (
    id BIGSERIAL PRIMARY KEY,
    submission_id BIGINT NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    test_case_id BIGINT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    verdict TEXT NOT NULL,
    runtime_ms INTEGER DEFAULT NULL,
    memory_kb INTEGER DEFAULT NULL,
    stderr_excerpt TEXT DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT submission_test_results_verdict_check
        CHECK (verdict IN ('AC', 'WA', 'TLE', 'MLE', 'RE', 'CE')),

    CONSTRAINT submission_test_results_unique_case
        UNIQUE (submission_id, test_case_id),

    CONSTRAINT submission_test_results_unique_idx
        UNIQUE (submission_id, idx)
);
