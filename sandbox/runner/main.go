//go:build linux

package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	verdictOK  = "OK"
	verdictTLE = "TLE"
	verdictMLE = "MLE"
	verdictRE  = "RE"

	stderrExcerptBytes = 2048
)

type result struct {
	Verdict         string `json:"verdict"`
	RuntimeMS       int64  `json:"runtime_ms"`
	MemoryKB        int64  `json:"memory_kb"`
	ExitCode        int    `json:"exit_code"`
	Signal          string `json:"signal,omitempty"`
	StderrExcerpt   string `json:"stderr_excerpt"`
	OutputTruncated bool   `json:"output_truncated"`
}

func main() {
	input := flag.String("input", "/work/input.txt", "file fed to the program as stdin")
	output := flag.String("output", "/work/output.txt", "file capturing the program's stdout")
	resultPath := flag.String("result", "/work/result.json", "file to write the result JSON")
	wallMS := flag.Int("wall-ms", 2000, "wall-clock limit in milliseconds")
	memKB := flag.Int("mem-kb", 262144, "memory limit in KB, for MLE classification")
	outputKB := flag.Int("output-kb", 1024, "max captured stdout in KB")
	flag.Parse()

	cmd := flag.Args()
	if len(cmd) == 0 {
		writeResult(*resultPath, result{Verdict: verdictRE, StderrExcerpt: "no command given"})
		return
	}

	writeResult(*resultPath, run(cmd, *input, *output, *wallMS, *memKB, *outputKB))
}

func run(cmdArgs []string, inputPath, outputPath string, wallMS, memKB, outputKB int) result {
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if in, err := os.Open(inputPath); err == nil {
		cmd.Stdin = in
		defer in.Close()
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return result{Verdict: verdictRE, StderrExcerpt: "cannot create output file: " + err.Error()}
	}
	defer outFile.Close()

	stdout := &cappedWriter{w: outFile, limit: outputKB * 1024}
	stderr := &cappedBuffer{limit: stderrExcerptBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return result{Verdict: verdictRE, StderrExcerpt: "start failed: " + err.Error()}
	}

	timedOut := false
	timer := time.AfterFunc(time.Duration(wallMS)*time.Millisecond, func() {
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	_ = cmd.Wait()
	timer.Stop()
	elapsed := time.Since(start)

	var maxRSSKB int64
	exitCode := 0
	signal := ""
	if st := cmd.ProcessState; st != nil {
		if ru, ok := st.SysUsage().(*syscall.Rusage); ok {
			maxRSSKB = int64(ru.Maxrss)
		}
		if ws, ok := st.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				exitCode = -1
				signal = ws.Signal().String()
			} else {
				exitCode = ws.ExitStatus()
			}
		}
	}

	verdict := verdictOK
	switch {
	case timedOut:
		verdict = verdictTLE
	case memKB > 0 && maxRSSKB >= int64(memKB):
		verdict = verdictMLE
	case exitCode != 0:
		verdict = verdictRE
	}

	return result{
		Verdict:         verdict,
		RuntimeMS:       elapsed.Milliseconds(),
		MemoryKB:        maxRSSKB,
		ExitCode:        exitCode,
		Signal:          signal,
		StderrExcerpt:   stderr.String(),
		OutputTruncated: stdout.truncated,
	}
}

func writeResult(path string, r result) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		data = []byte(`{"verdict":"RE","stderr_excerpt":"result marshal failed"}`)
	}
	_ = os.WriteFile(path, data, 0o644)
	_, _ = os.Stdout.Write(append(data, '\n'))
}

type cappedWriter struct {
	w         io.Writer
	limit     int
	written   int
	truncated bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	room := c.limit - c.written
	if room <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		c.truncated = true
		n, err := c.w.Write(p[:room])
		c.written += n
		return len(p), err
	}
	n, err := c.w.Write(p)
	c.written += n
	return n, err
}

type cappedBuffer struct {
	limit int
	buf   []byte
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.limit - len(c.buf); room > 0 {
		if room > len(p) {
			room = len(p)
		}
		c.buf = append(c.buf, p[:room]...)
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return string(c.buf) }
