package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// memHeadroomMB is added to the requested memory for the container's hard cap,
// leaving room for the interpreter/toolchain. MLE is decided by the runner
// comparing peak RSS to the real limit, not by an OOM kill.
const memHeadroomMB = 128

type DockerSandbox struct {
	dockerBin string
}

func NewDockerSandbox(dockerBin string) *DockerSandbox {
	if dockerBin == "" {
		dockerBin = "docker"
	}
	return &DockerSandbox{dockerBin: dockerBin}
}

type runnerResult struct {
	Verdict         string `json:"verdict"`
	RuntimeMS       int64  `json:"runtime_ms"`
	MemoryKB        int64  `json:"memory_kb"`
	StderrExcerpt   string `json:"stderr_excerpt"`
	OutputTruncated bool   `json:"output_truncated"`
}

func (d *DockerSandbox) Exec(ctx context.Context, req ExecRequest) (Output, error) {
	if err := os.WriteFile(filepath.Join(req.HostDir, "input.txt"), []byte(req.Input), 0o644); err != nil {
		return Output{}, err
	}

	memMB := req.MemKB/1024 + memHeadroomMB
	args := []string{
		"run", "--rm",
		"--network", "none",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=512m",
		"--memory", fmt.Sprintf("%dm", memMB),
		"--memory-swap", fmt.Sprintf("%dm", memMB),
		"--cpus", "1",
		"--pids-limit", "128",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "10001:10001",
		"-v", filepath.ToSlash(req.HostDir) + ":/work",
	}
	for _, k := range sortedKeys(req.Env) {
		args = append(args, "-e", k+"="+req.Env[k])
	}
	args = append(args,
		req.Image,
		"-input", "/work/input.txt",
		"-output", "/work/output.txt",
		"-result", "/work/result.json",
		"-wall-ms", strconv.Itoa(req.WallMS),
		"-mem-kb", strconv.Itoa(req.MemKB),
		"-output-kb", strconv.Itoa(req.OutputKB),
		"--",
	)
	args = append(args, req.Cmd...)

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, d.dockerBin, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Output{}, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	resData, err := os.ReadFile(filepath.Join(req.HostDir, "result.json"))
	if err != nil {
		return Output{}, fmt.Errorf("read result.json: %w", err)
	}
	var rr runnerResult
	if err := json.Unmarshal(resData, &rr); err != nil {
		return Output{}, fmt.Errorf("parse result.json: %w", err)
	}
	stdout, _ := os.ReadFile(filepath.Join(req.HostDir, "output.txt"))

	return Output{
		Verdict:         rr.Verdict,
		Stdout:          string(stdout),
		RuntimeMS:       int(rr.RuntimeMS),
		MemoryKB:        int(rr.MemoryKB),
		StderrExcerpt:   rr.StderrExcerpt,
		OutputTruncated: rr.OutputTruncated,
	}, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
