CREATE TABLE test_cases (
    id BIGSERIAL PRIMARY KEY, 
    problem_id BIGINT NOT NULL REFERENCES problems(id) ON DELETE CASCADE, 
    idx INTEGER NOT NULL UNIQUE, 
    input TEXT NOT NULL, 
    expected_output TEXT NOT NULL, 
    is_sample BOOLEAN NOT NULL DEFAULT FALSE, 
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (problem_id,idx)
);
