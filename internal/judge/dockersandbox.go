package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// memHeadroomMB is added to a problem's memory limit for the container's hard
// cap, leaving room for the interpreter itself. MLE is decided by the runner
// comparing peak RSS to the real limit, not by an OOM kill.
const memHeadroomMB = 128

type langSpec struct {
	file string
	cmd  []string
}

var languages = map[string]langSpec{
	"python": {file: "solution.py", cmd: []string{"python3", "/work/solution.py"}},
}

type DockerSandbox struct {
	image     string
	dockerBin string
	workRoot  string
}

func NewDockerSandbox(image, dockerBin, workRoot string) *DockerSandbox {
	if dockerBin == "" {
		dockerBin = "docker"
	}
	return &DockerSandbox{image: image, dockerBin: dockerBin, workRoot: workRoot}
}

type runnerResult struct {
	Verdict         string `json:"verdict"`
	RuntimeMS       int64  `json:"runtime_ms"`
	MemoryKB        int64  `json:"memory_kb"`
	StderrExcerpt   string `json:"stderr_excerpt"`
	OutputTruncated bool   `json:"output_truncated"`
}

func (d *DockerSandbox) Run(ctx context.Context, spec Spec) (Output, error) {
	lang, ok := languages[spec.Language]
	if !ok {
		return Output{}, fmt.Errorf("unsupported language: %q", spec.Language)
	}

	dir, err := os.MkdirTemp(d.workRoot, "gospoc-judge-")
	if err != nil {
		return Output{}, err
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, lang.file), []byte(spec.Source), 0o644); err != nil {
		return Output{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "input.txt"), []byte(spec.Input), 0o644); err != nil {
		return Output{}, err
	}

	memMB := spec.MemKB/1024 + memHeadroomMB
	args := []string{
		"run", "--rm",
		"--network", "none",
		"--read-only",
		"--memory", fmt.Sprintf("%dm", memMB),
		"--memory-swap", fmt.Sprintf("%dm", memMB),
		"--cpus", "1",
		"--pids-limit", "64",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "10001:10001",
		"-v", filepath.ToSlash(dir) + ":/work",
		d.image,
		"-input", "/work/input.txt",
		"-output", "/work/output.txt",
		"-result", "/work/result.json",
		"-wall-ms", strconv.Itoa(spec.WallMS),
		"-mem-kb", strconv.Itoa(spec.MemKB),
		"-output-kb", strconv.Itoa(spec.OutputKB),
		"--",
	}
	args = append(args, lang.cmd...)

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, d.dockerBin, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Output{}, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	resData, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		return Output{}, fmt.Errorf("read result.json: %w", err)
	}
	var rr runnerResult
	if err := json.Unmarshal(resData, &rr); err != nil {
		return Output{}, fmt.Errorf("parse result.json: %w", err)
	}
	stdout, _ := os.ReadFile(filepath.Join(dir, "output.txt"))

	return Output{
		Verdict:         rr.Verdict,
		Stdout:          string(stdout),
		RuntimeMS:       int(rr.RuntimeMS),
		MemoryKB:        int(rr.MemoryKB),
		StderrExcerpt:   rr.StderrExcerpt,
		OutputTruncated: rr.OutputTruncated,
	}, nil
}
