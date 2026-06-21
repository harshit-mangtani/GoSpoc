package submission

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/harshit-mangtani/GoSpoc/internal/auth"
	"github.com/harshit-mangtani/GoSpoc/internal/queue"
	"github.com/jackc/pgx/v5"
)

const maxSourceBytes = 64 * 1024

type Handler struct {
	submissions *Repository
	queue       queue.Queue
	logger      *slog.Logger
}

func NewHandler(submissions *Repository, q queue.Queue, logger *slog.Logger) *Handler {
	return &Handler{
		submissions: submissions,
		queue:       q,
		logger:      logger,
	}
}

type createRequest struct {
	ProblemID int64  `json:"problem_id"`
	Language  string `json:"language"`
	Source    string `json:"source"`
}

type submissionResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ProblemID int64     `json:"problem_id"`
	Language  string    `json:"language"`
	Source    string    `json:"source"`
	Status    string    `json:"status"`
	Verdict   *string   `json:"verdict"`
	RuntimeMS *int      `json:"runtime_ms"`
	MemoryKB  *int      `json:"memory_kb"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type listResponse struct {
	Submissions []submissionResponse `json:"submissions"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	req.Language = strings.TrimSpace(strings.ToLower(req.Language))
	if req.ProblemID <= 0 {
		http.Error(w, "problem_id is required", http.StatusBadRequest)
		return
	}

	if !allowedLanguage(req.Language) {
		http.Error(w, "language is not supported", http.StatusBadRequest)
		return
	}

	if req.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}

	if len([]byte(req.Source)) > maxSourceBytes {
		http.Error(w, "source is too large", http.StatusBadRequest)
		return
	}

	s, err := h.submissions.Create(r.Context(), userID, req.ProblemID, req.Language, req.Source)
	if err != nil {
		http.Error(w, "failed to create submission", http.StatusInternalServerError)
		return
	}

	// Enqueue is best-effort: the row is already persisted as "queued", so if
	// this fails the sweeper will re-enqueue it. Don't fail the request.
	if err := h.queue.Enqueue(r.Context(), queue.Job{SubmissionID: s.ID}); err != nil {
		h.logger.Error("failed to enqueue submission", "submission_id", s.ID, "error", err)
	}

	writeJSON(w, http.StatusAccepted, toResponse(s))
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid submission id", http.StatusBadRequest)
		return
	}

	s, err := h.submissions.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "submission not found", http.StatusNotFound)
			return
		}

		http.Error(w, "failed to get submission", http.StatusInternalServerError)
		return
	}

	if s.UserID != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, toResponse(s))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var (
		submissions []Submission
		err         error
	)

	problemIDParam := r.URL.Query().Get("problem_id")
	if problemIDParam == "" {
		submissions, err = h.submissions.ListByUser(r.Context(), userID)
	} else {
		problemID, parseErr := strconv.ParseInt(problemIDParam, 10, 64)
		if parseErr != nil || problemID <= 0 {
			http.Error(w, "invalid problem_id", http.StatusBadRequest)
			return
		}

		submissions, err = h.submissions.ListByUserAndProblem(r.Context(), userID, problemID)
	}

	if err != nil {
		http.Error(w, "failed to list submissions", http.StatusInternalServerError)
		return
	}

	res := listResponse{
		Submissions: make([]submissionResponse, 0, len(submissions)),
	}

	for _, s := range submissions {
		res.Submissions = append(res.Submissions, toResponse(s))
	}

	writeJSON(w, http.StatusOK, res)
}

func allowedLanguage(language string) bool {
	switch language {
	case "python", "go":
		return true
	default:
		return false
	}
}

func toResponse(s Submission) submissionResponse {
	return submissionResponse{
		ID:        s.ID,
		UserID:    s.UserID,
		ProblemID: s.ProblemID,
		Language:  s.Language,
		Source:    s.Source,
		Status:    s.Status,
		Verdict:   s.Verdict,
		RuntimeMS: s.RuntimeMS,
		MemoryKB:  s.MemoryKB,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
