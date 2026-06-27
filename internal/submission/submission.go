package submission

import "time"

type Submission struct {
	ID        int64
	UserID    int64
	ProblemID int64
	Language  string
	Source    string
	Status       string
	Verdict      *string
	RuntimeMS    *int
	MemoryKB     *int
	CompileError *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
