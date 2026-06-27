package submission

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (r *Repository) Create(ctx context.Context, userID, problemID int64, language, source string) (Submission, error) {
	var s Submission

	err := r.pool.QueryRow(ctx, `
		INSERT INTO submissions (user_id, problem_id, language, source)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, problem_id, language, source, status, verdict, runtime_ms, memory_kb, created_at, updated_at
	`, userID, problemID, language, source).Scan(
		&s.ID,
		&s.UserID,
		&s.ProblemID,
		&s.Language,
		&s.Source,
		&s.Status,
		&s.Verdict,
		&s.RuntimeMS,
		&s.MemoryKB,
		&s.CreatedAt,
		&s.UpdatedAt,
	)

	if err != nil {
		return Submission{}, err
	}

	return s, nil
}

// MarkRunning claims a queued submission. The status guard makes it idempotent:
// ok is false if it was already claimed or isn't queued.
func (r *Repository) MarkRunning(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE submissions
		SET status = 'running', updated_at = NOW()
		WHERE id = $1 AND status = 'queued'
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// MarkDone records the verdict/runtime/memory, guarded so it only lands on a
// claimed (running) submission.
func (r *Repository) MarkDone(ctx context.Context, id int64, verdict string, runtimeMS, memoryKB int) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE submissions
		SET status = 'done', verdict = $2, runtime_ms = $3, memory_kb = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'running'
	`, id, verdict, runtimeMS, memoryKB)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// MarkFailed marks a running submission as failed (judging infrastructure error,
// not a code verdict).
func (r *Repository) MarkFailed(ctx context.Context, id int64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE submissions
		SET status = 'failed', updated_at = NOW()
		WHERE id = $1 AND status = 'running'
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

type TestResult struct {
	SubmissionID  int64
	TestCaseID    int64
	Idx           int
	Verdict       string
	RuntimeMS     int
	MemoryKB      int
	StderrExcerpt string
}

func (r *Repository) SaveTestResult(ctx context.Context, tr TestResult) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO submission_test_results
			(submission_id, test_case_id, idx, verdict, runtime_ms, memory_kb, stderr_excerpt)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (submission_id, test_case_id) DO NOTHING
	`, tr.SubmissionID, tr.TestCaseID, tr.Idx, tr.Verdict, tr.RuntimeMS, tr.MemoryKB, tr.StderrExcerpt)
	return err
}

// SetCompileError stores (truncated) compiler output on a submission. Status is
// left to the caller's MarkDone/MarkFailed.
func (r *Repository) SetCompileError(ctx context.Context, id int64, msg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE submissions SET compile_error = $2 WHERE id = $1
	`, id, msg)
	return err
}

func (r *Repository) FindByID(ctx context.Context, id int64) (Submission, error) {
	var s Submission

	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, problem_id, language, source, status, verdict, runtime_ms, memory_kb, compile_error, created_at, updated_at
		FROM submissions
		WHERE id = $1
	`, id).Scan(
		&s.ID,
		&s.UserID,
		&s.ProblemID,
		&s.Language,
		&s.Source,
		&s.Status,
		&s.Verdict,
		&s.RuntimeMS,
		&s.MemoryKB,
		&s.CompileError,
		&s.CreatedAt,
		&s.UpdatedAt,
	)

	if err != nil {
		return Submission{}, err
	}

	return s, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID int64) ([]Submission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, problem_id, language, source, status, verdict, runtime_ms, memory_kb, created_at, updated_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	submissions := make([]Submission, 0)
	for rows.Next() {
		var s Submission

		if err := rows.Scan(
			&s.ID,
			&s.UserID,
			&s.ProblemID,
			&s.Language,
			&s.Source,
			&s.Status,
			&s.Verdict,
			&s.RuntimeMS,
			&s.MemoryKB,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}

		submissions = append(submissions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return submissions, nil
}

// ListStaleQueued returns the ids of submissions still in "queued" status
// whose last update is older than the given cutoff. These are candidates for
// re-enqueue: either their enqueue failed, or the message was lost before a
// worker picked it up.
func (r *Repository) ListStaleQueued(ctx context.Context, olderThan time.Duration) ([]int64, error) {
	cutoff := time.Now().Add(-olderThan)

	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM submissions
		WHERE status = 'queued' AND updated_at < $1
		ORDER BY id
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

func (r *Repository) ListByUserAndProblem(ctx context.Context, userID, problemID int64) ([]Submission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, problem_id, language, source, status, verdict, runtime_ms, memory_kb, created_at, updated_at
		FROM submissions
		WHERE user_id = $1 AND problem_id = $2
		ORDER BY id DESC
	`, userID, problemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	submissions := make([]Submission, 0)
	for rows.Next() {
		var s Submission

		if err := rows.Scan(
			&s.ID,
			&s.UserID,
			&s.ProblemID,
			&s.Language,
			&s.Source,
			&s.Status,
			&s.Verdict,
			&s.RuntimeMS,
			&s.MemoryKB,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, err
		}

		submissions = append(submissions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return submissions, nil
}
