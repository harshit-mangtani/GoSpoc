package user

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

func (r *Repository) Create(ctx context.Context, email, passwordHash string, role string) (User, error) {
	var u User

	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, email, role
	`, email, passwordHash, role).Scan(&u.ID, &u.Email, &u.Role)
	if err != nil {
		return User{}, err
	}

	return u, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (User, error) {
	var u User

	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role)

	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (User, error) {
	var u User

	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, role
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role)

	if err != nil {
		return User{}, err
	}
	return u, nil
}
