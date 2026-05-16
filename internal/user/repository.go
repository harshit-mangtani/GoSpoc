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

func (r *Repository) Create(ctx context.Context, email, passwordHash string) (User, error){
	var u User

	err:= r.pool.QueryRow(ctx,`
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email
		`, email, passwordHash).Scan(&u.ID,&u.Email)
	if err != nil {
		return User{},err
	}

	return u,nil
}