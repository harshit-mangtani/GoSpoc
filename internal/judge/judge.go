package judge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	CE  = "CE"

	compileErrorMax = 4096
)

// ExecRequest runs one command inside a locked-down container with HostDir
// mounted at /work and Input fed on stdin.
type ExecRequest struct {
	Image    string
	HostDir  string
	Cmd      []string
	Input    string
	Env      map[string]string
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
	Exec(ctx context.Context, req ExecRequest) (Output, error)
}

type Config struct {
	PythonImage   string
	GoImage       string
	WorkDir       string
	OutputKB      int
	CompileWallMS int
	CompileMemKB  int
}

type langConfig struct {
	image      string
	sourceFile string
	compileCmd []string // nil for interpreted languages
	compileEnv map[string]string
	runCmd     []string
}

type Result struct {
	Verdict   string
	RuntimeMS int
	MemoryKB  int
}

type Judge struct {
	problems    *problem.Repository
	testcases   *testcase.Repository
	submissions *submission.Repository
	sandbox     Sandbox
	logger      *slog.Logger

	languages     map[string]langConfig
	workDir       string
	outputKB      int
	compileWallMS int
	compileMemKB  int
}

func New(problems *problem.Repository, testcases *testcase.Repository, submissions *submission.Repository, sandbox Sandbox, logger *slog.Logger, cfg Config) *Judge {
	if cfg.OutputKB <= 0 {
		cfg.OutputKB = 1024
	}
	if cfg.CompileWallMS <= 0 {
		cfg.CompileWallMS = 10000
	}
	if cfg.CompileMemKB <= 0 {
		cfg.CompileMemKB = 512 * 1024
	}
	return &Judge{
		problems:    problems,
		testcases:   testcases,
		submissions: submissions,
		sandbox:     sandbox,
		logger:      logger,
		languages: map[string]langConfig{
			"python": {
				image:      cfg.PythonImage,
				sourceFile: "solution.py",
				runCmd:     []string{"python3", "/work/solution.py"},
			},
			"go": {
				image:      cfg.GoImage,
				sourceFile: "solution.go",
				compileCmd: []string{"sh", "-c", "cp -a /opt/gocache /tmp/gocache && go build -o /work/prog /work/solution.go"},
				compileEnv: map[string]string{"GOCACHE": "/tmp/gocache", "HOME": "/tmp"},
				runCmd:     []string{"/work/prog"},
			},
		},
		workDir:       cfg.WorkDir,
		outputKB:      cfg.OutputKB,
		compileWallMS: cfg.CompileWallMS,
		compileMemKB:  cfg.CompileMemKB,
	}
}

// Run judges one submission: compile (for compiled languages) then run every
// test case, stopping at the first non-AC.
func (j *Judge) Run(ctx context.Context, submissionID int64) (Result, error) {
	sub, err := j.submissions.FindByID(ctx, submissionID)
	if err != nil {
		return Result{}, fmt.Errorf("load submission: %w", err)
	}
	lang, ok := j.languages[sub.Language]
	if !ok {
		return Result{}, fmt.Errorf("unsupported language: %q", sub.Language)
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

	dir, err := os.MkdirTemp(j.workDir, "gospoc-judge-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, lang.sourceFile), []byte(sub.Source), 0o644); err != nil {
		return Result{}, err
	}

	if lang.compileCmd != nil {
		out, err := j.sandbox.Exec(ctx, ExecRequest{
			Image: lang.image, HostDir: dir, Cmd: lang.compileCmd, Env: lang.compileEnv,
			WallMS: j.compileWallMS, MemKB: j.compileMemKB, OutputKB: j.outputKB,
		})
		if err != nil {
			return Result{}, fmt.Errorf("compile: %w", err)
		}
		if out.Verdict != "OK" {
			msg := out.StderrExcerpt
			if out.Verdict == TLE {
				msg = "compilation timed out"
			}
			if err := j.submissions.SetCompileError(ctx, submissionID, truncate(msg, compileErrorMax)); err != nil {
				j.logger.Error("store compile error failed", "submission_id", submissionID, "error", err)
			}
			return Result{Verdict: CE}, nil
		}
	}

	final := AC
	var maxRT, maxMem int
	for _, tc := range cases {
		out, err := j.sandbox.Exec(ctx, ExecRequest{
			Image: lang.image, HostDir: dir, Cmd: lang.runCmd, Input: tc.Input,
			WallMS: prob.TimeLimitMS, MemKB: prob.MemoryLimitKB, OutputKB: j.outputKB,
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

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
