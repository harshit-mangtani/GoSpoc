package problem

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, slug, title, statement string, timeLimitMS, memoryLimitKB int) (Problem, error) {
	var p Problem

	err := r.pool.QueryRow(ctx, `
		INSERT INTO problems (slug, title, statement, time_limit_ms, memory_limit_kb)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, slug, title, statement, time_limit_ms, memory_limit_kb
	`, slug, title, statement, timeLimitMS, memoryLimitKB).Scan(
		&p.ID,
		&p.Slug,
		&p.Title,
		&p.Statement,
		&p.TimeLimitMS,
		&p.MemoryLimitKB,
	)

	if err != nil {
		return Problem{}, err
	}

	return p, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (Problem, error) {
	var p Problem
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, statement, time_limit_ms, memory_limit_kb
		FROM problems
		WHERE id = $1
	`, id).Scan(
		&p.ID,
		&p.Slug,
		&p.Title,
		&p.Statement,
		&p.TimeLimitMS,
		&p.MemoryLimitKB,
	)
	if err != nil {
		return Problem{}, err
	}
	return p, nil
}

func (r *Repository) List(ctx context.Context) ([]Problem, error) {

	var problems []Problem

	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, title, statement, time_limit_ms, memory_limit_kb
		FROM problems
		ORDER BY id DESC
	`)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var p Problem

		if err := rows.Scan(
			&p.ID,
			&p.Slug,
			&p.Title,
			&p.Statement,
			&p.TimeLimitMS,
			&p.MemoryLimitKB,
		); err != nil {
			return nil, err
		}

		problems = append(problems, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return problems, nil

}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (Problem, error) {
	var problem Problem
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, statement, time_limit_ms, memory_limit_kb
		FROM problems
		WHERE slug = $1
	`, slug).Scan(
		&problem.ID,
		&problem.Slug,
		&problem.Title,
		&problem.Statement,
		&problem.TimeLimitMS,
		&problem.MemoryLimitKB,
	)

	if err != nil {
		return Problem{}, err
	}
	return problem, nil
}
