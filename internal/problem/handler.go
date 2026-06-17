package problem

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Handler struct {
	problems *Repository
}

func NewHandler(problems *Repository) *Handler {
	return &Handler{
		problems: problems,
	}
}

type createRequest struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Statement     string `json:"statement"`
	TimeLimitMS   int    `json:"time_limit_ms"`
	MemoryLimitKB int    `json:"memory_limit_kb"`
}

type createResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Statement     string `json:"statement"`
	TimeLimitMS   int    `json:"time_limit_ms"`
	MemoryLimitKB int    `json:"memory_limit_kb"`
}

type listProblemResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Statement     string `json:"statement"`
	TimeLimitMS   int    `json:"time_limit_ms"`
	MemoryLimitKB int    `json:"memory_limit_kb"`
}

type listResponse struct {
	Problems []listProblemResponse `json:"problems"`
}

type GetProblemResponse struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Statement     string `json:"statement"`
	TimeLimitMS   int    `json:"time_limit_ms"`
	MemoryLimitKB int    `json:"memory_limit_kb"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	req.Title = strings.TrimSpace(req.Title)
	req.Statement = strings.TrimSpace(req.Statement)

	if req.Slug == "" || req.Title == "" || req.Statement == "" {
		http.Error(w, "slug, title and statement are required", http.StatusBadRequest)
		return
	}

	if req.TimeLimitMS <= 0 || req.MemoryLimitKB <= 0 {
		http.Error(w, "time_limit_ms and memory_limit_kb must be positive", http.StatusBadRequest)
		return
	}

	p, err := h.problems.Create(r.Context(),
		req.Slug,
		req.Title,
		req.Statement,
		req.TimeLimitMS,
		req.MemoryLimitKB)
	if err != nil {
		http.Error(w, "failed to create problem", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(createResponse{
		ID:            p.ID,
		Slug:          p.Slug,
		Title:         p.Title,
		Statement:     p.Statement,
		TimeLimitMS:   p.TimeLimitMS,
		MemoryLimitKB: p.MemoryLimitKB,
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	problems, err := h.problems.List(r.Context())

	if err != nil {
		http.Error(w, "failed to list problems", http.StatusInternalServerError)
		return
	}

	res := listResponse{
		Problems: make([]listProblemResponse, 0, len(problems)),
	}

	for _, p := range problems {
		res.Problems = append(res.Problems, listProblemResponse{
			ID:            p.ID,
			Slug:          p.Slug,
			Title:         p.Title,
			TimeLimitMS:   p.TimeLimitMS,
			MemoryLimitKB: p.MemoryLimitKB,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

func (h *Handler) GetProblem(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(strings.ToLower(r.PathValue("slug")))
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	problem, err := h.problems.GetBySlug(r.Context(), slug)

	if err != nil {
		http.Error(w, "failed to get problem", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(GetProblemResponse{
		ID:            problem.ID,
		Slug:          problem.Slug,
		Title:         problem.Title,
		Statement:     problem.Statement,
		TimeLimitMS:   problem.TimeLimitMS,
		MemoryLimitKB: problem.MemoryLimitKB,
	})
}
