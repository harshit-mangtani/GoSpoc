package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/harshit-mangtani/GoSpoc/internal/user"
	"github.com/jackc/pgx/v5/pgconn"

)

type Handler struct {
	users *user.Repository
}

func NewHandler(users *user.Repository) *Handler {
	return &Handler{
		users: users,
	}
}

type signupRequest struct {
	Email string `json:"email"`
	Password string `json:"password"`
}

type signupResponse struct {
	ID int64 `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) Signup(w http.ResponseWriter,r *http.Request){
	var req signupRequest

	if err:= json.NewDecoder(r.Body).Decode(&req); err!=nil{
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	email:=strings.TrimSpace(strings.ToLower(req.Email))
	password:= req.Password

	if email == "" || password == "" {
		http.Error(w, "email and password cant be empty", http.StatusBadRequest)
		return
	}

	if len(password) <8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	passwordHash, err:= HashPassword(password)

	if err!=nil{
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	u,err := h.users.Create(r.Context(), email, passwordHash)
	if err!= nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "user with this email already exists",http.StatusConflict)
	        return
		}
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(signupResponse{
		ID: u.ID,
		Email: u.Email,
	})
}