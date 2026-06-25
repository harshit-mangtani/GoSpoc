# Sandbox

The in-container **runner** executes one user program under limits and writes a
result JSON. The worker (Phase 8) will build a per-submission work directory,
run this image against it, and read the result back.

## Runner contract

Entrypoint: `/usr/local/bin/runner`. It runs the command given after the flags,
feeding `-input` as stdin and capturing stdout to `-output`.

```
runner \
  -input   /work/input.txt \
  -output  /work/output.txt \
  -result  /work/result.json \
  -wall-ms   2000 \
  -mem-kb    262144 \
  -output-kb 1024 \
  -- python3 /work/solution.py
```

### Result JSON (`-result`)

```json
{
  "verdict": "OK",           // OK | TLE | MLE | RE  (never AC/WA — that's the judge's job)
  "runtime_ms": 41,
  "memory_kb": 9152,          // peak RSS (rusage.Maxrss)
  "exit_code": 0,             // -1 if killed by a signal
  "signal": "",               // signal name when exit_code == -1
  "stderr_excerpt": "",       // first 2 KB of stderr
  "output_truncated": false   // true if stdout exceeded -output-kb
}
```

Verdict precedence: `TLE` (wall-clock exceeded) > `MLE` (peak RSS ≥ `-mem-kb`) >
`RE` (non-zero exit or signal) > `OK`. The runner always exits 0 — the outcome
lives in the result file, not the process exit code.

**Division of labor:** the runner enforces the wall-clock timeout (it kills the
whole process group) and the output-size cap, and *measures* memory. Memory,
CPU, and PID limits are enforced by Docker via the run flags below.

## Build

```sh
docker build -f sandbox/Dockerfile.python -t gospoc-sandbox-python .
```

## Hardened `docker run` flags

Every judged run uses these. The rootfs is read-only; only `/work` (a per-run
mount) is writable.

```sh
docker run --rm \
  --network none \                     # no network access
  --read-only \                        # immutable root filesystem
  --memory 256m --memory-swap 256m \   # hard memory cap, no swap
  --cpus 1 \                           # CPU cap
  --pids-limit 64 \                    # cap process/thread count (fork bombs)
  --cap-drop ALL \                     # drop all Linux capabilities
  --security-opt no-new-privileges \   # no privilege escalation
  --user 10001:10001 \                 # non-root
  -v "$WORK:/work" \                   # per-submission dir (solution + input; result written back)
  gospoc-sandbox-python \
  -input /work/input.txt -output /work/output.txt -result /work/result.json \
  -wall-ms 2000 -mem-kb 262144 -output-kb 1024 -- python3 /work/solution.py
```
