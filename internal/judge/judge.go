package judge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/harshit-mangtani/GoSpoc/internal/problem"
	"github.com/harshit-mangtani/GoSpoc/internal/submission"
	"github.com/harshit-mangtani/GoSpoc/internal/testcase"
)

const (
	AC  = "AC"
	WA  = "WA"
	TLE = "TLE"
	MLE = "MLE"
	RE  = "RE"
)

type Spec struct {
	Language string
	Source   string
	Input    string
	WallMS   int
	MemKB    int
	OutputKB int
}

// Output is the runner's view of a single execution. Verdict is one of
// OK/TLE/MLE/RE; the judge turns OK into AC or WA by comparing Stdout.
type Output struct {
	Verdict         string
	Stdout          string
	RuntimeMS       int
	MemoryKB        int
	StderrExcerpt   string
	OutputTruncated bool
}

type Sandbox interface {
	Run(ctx context.Context, spec Spec) (Output, error)
}

type ProblemStore interface {
	GetByID(ctx context.Context, id int64) (problem.Problem, error)
}

type TestCaseStore interface {
	ListByProblem(ctx context.Context, problemID int64) ([]testcase.TestCase, error)
}

type SubmissionStore interface {
	FindByID(ctx context.Context, id int64) (submission.Submission, error)
	SaveTestResult(ctx context.Context, tr submission.TestResult) error
}

type Result struct {
	Verdict   string
	RuntimeMS int
	MemoryKB  int
}

type Judge struct {
	problems    ProblemStore
	testcases   TestCaseStore
	submissions SubmissionStore
	sandbox     Sandbox
	logger      *slog.Logger
	outputKB    int
}

func New(problems ProblemStore, testcases TestCaseStore, submissions SubmissionStore, sandbox Sandbox, logger *slog.Logger, outputKB int) *Judge {
	if outputKB <= 0 {
		outputKB = 1024
	}
	return &Judge{
		problems:    problems,
		testcases:   testcases,
		submissions: submissions,
		sandbox:     sandbox,
		logger:      logger,
		outputKB:    outputKB,
	}
}

// Run judges one submission against every test case, stopping at the first
// non-AC. It records each executed test case's result and returns the aggregate.
func (j *Judge) Run(ctx context.Context, submissionID int64) (Result, error) {
	sub, err := j.submissions.FindByID(ctx, submissionID)
	if err != nil {
		return Result{}, fmt.Errorf("load submission: %w", err)
	}
	prob, err := j.problems.GetByID(ctx, sub.ProblemID)
	if err != nil {
		return Result{}, fmt.Errorf("load problem: %w", err)
	}
	cases, err := j.testcases.ListByProblem(ctx, sub.ProblemID)
	if err != nil {
		return Result{}, fmt.Errorf("load test cases: %w", err)
	}
	if len(cases) == 0 {
		return Result{}, fmt.Errorf("problem %d has no test cases", sub.ProblemID)
	}

	final := AC
	var maxRT, maxMem int
	for _, tc := range cases {
		out, err := j.sandbox.Run(ctx, Spec{
			Language: sub.Language,
			Source:   sub.Source,
			Input:    tc.Input,
			WallMS:   prob.TimeLimitMS,
			MemKB:    prob.MemoryLimitKB,
			OutputKB: j.outputKB,
		})
		if err != nil {
			return Result{}, fmt.Errorf("sandbox run (test %d): %w", tc.Idx, err)
		}

		verdict := classify(out, tc.ExpectedOutput)
		if out.RuntimeMS > maxRT {
			maxRT = out.RuntimeMS
		}
		if out.MemoryKB > maxMem {
			maxMem = out.MemoryKB
		}

		if err := j.submissions.SaveTestResult(ctx, submission.TestResult{
			SubmissionID:  submissionID,
			TestCaseID:    tc.ID,
			Idx:           tc.Idx,
			Verdict:       verdict,
			RuntimeMS:     out.RuntimeMS,
			MemoryKB:      out.MemoryKB,
			StderrExcerpt: out.StderrExcerpt,
		}); err != nil {
			j.logger.Error("save test result failed", "submission_id", submissionID, "test_case_id", tc.ID, "error", err)
		}

		if verdict != AC {
			final = verdict
			break
		}
	}

	return Result{Verdict: final, RuntimeMS: maxRT, MemoryKB: maxMem}, nil
}

func classify(out Output, expected string) string {
	switch out.Verdict {
	case "OK":
		if out.OutputTruncated {
			return WA
		}
		if normalize(out.Stdout) == normalize(expected) {
			return AC
		}
		return WA
	case TLE:
		return TLE
	case MLE:
		return MLE
	default:
		return RE
	}
}

// normalize makes output comparison insensitive to trailing whitespace and
// trailing blank lines, matching typical judge behaviour.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
