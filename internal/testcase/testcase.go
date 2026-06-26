package testcase

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TestCase struct {
	ID             int64
	ProblemID      int64
	Idx            int
	Input          string
	ExpectedOutput string
	IsSample       bool
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListByProblem(ctx context.Context, problemID int64) ([]TestCase, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, problem_id, idx, input, expected_output, is_sample
		FROM test_cases
		WHERE problem_id = $1
		ORDER BY idx
	`, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cases := make([]TestCase, 0)
	for rows.Next() {
		var t TestCase
		if err := rows.Scan(&t.ID, &t.ProblemID, &t.Idx, &t.Input, &t.ExpectedOutput, &t.IsSample); err != nil {
			return nil, err
		}
		cases = append(cases, t)
	}
	return cases, rows.Err()
}
