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

func (r *Repository) Get (ctx *context.Context)