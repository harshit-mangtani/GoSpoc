# Judge Platform — Roadmap & Memory Map

A LeetCode-style competitive programming backend, built in Go.
This file is the source of truth for plan + progress. Update checkboxes as work completes. Other agents should read this first.

---

## Project goals

- Backend in **Go**, hand-written for learning (no copy-paste of large blocks).
- Production-shaped from day one (real patterns, not toy code).
- Security-first for the code-execution path.
- Small steps. Each step should be understandable before moving to the next.

## Tech stack (locked in)

- **Language:** Go
- **DB:** Postgres
- **Cache / queue:** Redis (Streams + consumer groups for the job queue)
- **Sandbox:** Docker (Phase 7+); upgrade to nsjail/isolate later
- **Object storage:** local disk first, MinIO/S3 later
- **Auth:** JWT + argon2id password hashing
- **Migrations:** `golang-migrate` (decision pending in Phase 2)

## Target project layout (will grow into this)

```
cmd/
  api/           # HTTP server binary
  worker/        # judge worker binary
internal/
  auth/          # JWT, password hashing
  problem/       # problem domain
  submission/    # submission domain
  judge/         # sandbox runner glue, verdict logic
  queue/         # queue interface + Redis Streams impl
  storage/       # Postgres + Redis clients
  httpx/         # middleware (logging, recovery, request id)
  config/        # env loading
migrations/
sandbox/
  runner/        # tiny Go binary that runs INSIDE the sandbox container
  Dockerfile.python
  Dockerfile.go
```

---

## Phases

Legend: `[ ]` not started · `[~]` in progress · `[x]` done

### Phase 0 — Foundations
Goal: a working Go dev environment and an empty repo we can build on.

- [x] Install Go (latest stable) and verify `go version`
- [x] Install Docker Desktop and verify `docker run hello-world` 
- [x] `git init` in this directory, add `.gitignore` for Go
- [x] `go mod init github.com/harshit-mangtani/judge` (or chosen module path)
- [x] Create empty `cmd/api/main.go` that prints "hello"
- [x] `go run ./cmd/api` works

### Phase 1 — HTTP API skeleton
Goal: a real HTTP server with config, structured logging, and graceful shutdown.

- [x] Add a `config` package that reads env vars (port, log level)
- [x] Use `net/http` + `http.ServeMux` (standard library; no framework yet)
- [x] Add `slog` structured logging (JSON in prod, text in dev)
- [x] Add a `GET /healthz` endpoint that returns 200 OK
- [x] Implement graceful shutdown on SIGINT/SIGTERM (with `context`)
- [x] Add request-ID middleware (UUID per request, in logs + response header)
- [x] Add panic-recovery middleware

### Phase 2 — Database & users
Goal: Postgres connected, migrations running, users can sign up and log in.

- [x] Run Postgres locally via Docker Compose
- [x] Pick a migration tool (`golang-migrate`) and add `migrations/` dir
- [x] Migration 0001: `users` table (id, email, password_hash, created_at)
- [x] Add `pgx` driver and a `storage/postgres` package with a connection pool
- [x] `internal/auth`: argon2id hash + verify helpers
- [x] `POST /auth/signup` — validates input, creates user
- [x] `POST /auth/login` — verifies password, returns JWT
- [x] JWT auth middleware → puts `userID` in `context.Context`
- [x] `GET /me` (protected) returns the current user

### Phase 3 — Problems
Goal: problems can be stored and listed. No judging yet.

- [x] Migration 0002: `problems` table (id, slug, title, statement, time/memory limits)
- [x] Migration 0003: `test_cases` table (id, problem_id, idx, input, expected_output, is_sample)
- [x] `internal/problem` repo + service
- [x] `GET /problems` — list (paginated remain)
- [x] `GET /problems/{slug}` — detail (only sample test cases visible to user)
- [x] Admin-only `POST /problems` (gate by user role; add `role` column to users)
- [x] Seed script: insert one example problem ("two-sum") with a few test cases

### Phase 4 — Submission intake
Goal: user can submit code and get a `submission_id` back. No execution yet — verdict stays `queued`.

- [x] Migration 0004: `submissions` table (id, user_id, problem_id, language, source, status, verdict, runtime_ms, memory_kb, timestamps)
- [x] Migration 0005: `submission_test_results` table
- [x] `internal/submission` repo + service
- [x] `POST /submissions` (auth required) → writes row with `status=queued`, returns 202 + id
- [x] Input validation: language allow-list, source size cap (e.g. 64 KB)
- [x] `GET /submissions/{id}` — owner-only
- [x] `GET /submissions?problem_id=...` — own submissions (via `ListByUserAndProblem`)

### Phase 5 — Job queue (Redis Streams)
Goal: submissions get pushed onto a durable queue.

- [x] Run Redis locally via Docker Compose
- [x] Add `redis/go-redis` client, `storage/redis` package
- [x] Define `queue.Queue` interface (`Enqueue`, `Consume`, `Ack`, `Nack`) — fallible ops now return `error`; added `ErrNoMessage` sentinel
- [x] Implement `queue/redisstream` using Redis Streams + consumer groups (XADD / XREADGROUP / XACK)
- [x] On `POST /submissions`, after DB insert, enqueue the job (best-effort; sweeper is the backstop)
- [x] Sweeper: periodic check for `queued` rows older than N seconds and re-enqueue

### Phase 6 — Worker skeleton
Goal: a separate binary that consumes jobs and updates submissions — but uses a **fake** verdict for now.

- [x] `cmd/worker/main.go` — same config + logging conventions as API
- [x] Connect to Postgres + Redis
- [x] Consumer-group loop: read job, mark submission `running`, sleep 1s, mark `done` with verdict `AC`
- [x] Status transitions guarded by `WHERE status = 'queued'` (idempotent)
- [x] Graceful shutdown: finish current job, then exit
- [x] Bounded concurrency (N goroutines, configurable)

### Phase 7 — Sandbox: the runner contract
Goal: the in-container runner exists and we trust its output. Still no real judging from the worker yet.

- [x] `sandbox/runner/main.go` — small Go binary that:
  - reads stdin from a file
  - runs the user program with wall-clock + memory + output-size limits
  - writes a JSON result file: `{verdict, runtime_ms, memory_kb, exit_code, stderr_excerpt}`
- [x] `sandbox/Dockerfile.python` — base image with Python + the runner binary
- [x] Manual test: build image, `docker run` it with a known program, verify result JSON
- [x] Document the exact `docker run` flags we use (network none, read-only FS, mem/cpu/pids limits, cap-drop, no-new-privileges, non-root)

### Phase 8 — Real judging (Python only)
Goal: end-to-end. User submits Python → real verdict comes back.

- [x] `internal/judge` package: takes a submission, runs sandbox per test case
- [x] Worker swaps fake verdict for `judge.Run(submission)`
- [x] Per-test-case results written to `submission_test_results`
- [x] Verdict aggregation: stop on first non-AC, return that verdict
- [x] Map runner output → verdicts: AC, WA, TLE, MLE, RE
- [x] Manual test with our seeded "two-sum" problem

### Phase 9 — Compiled languages (Go)
Goal: support a language that needs a compile step.

- [x] `sandbox/Dockerfile.go` (named `Dockerfile.golang` — a `.go` file breaks `go build ./...`)
- [x] Two-stage execution: compile (own time/mem limit) → run
- [x] Compile failure → `CE` verdict; compiler stderr stored (truncated)
- [x] Worker chooses language config based on `submission.language`

### Phase 10 — Live updates
Goal: the client doesn't have to poll.

- [x] `GET /submissions/{id}/events` — Server-Sent Events stream
- [x] Worker publishes status changes to a Redis pub/sub channel
- [x] API subscribes, fans out to connected SSE clients
- [x] Decision later: SSE vs WebSocket — SSE is simpler, do it first (chose SSE)

### Phase 11 — Hardening
Goal: things you'd be embarrassed to ship without.

- [ ] Rate limiting on `POST /submissions` (per-user, Redis-backed token bucket)
- [ ] `Idempotency-Key` header support on `POST /submissions`
- [ ] Output-size cap enforced inside the runner (not just outside)
- [ ] Configurable per-language time/memory multipliers
- [ ] Pre-pull sandbox images on worker startup

### Phase 12 — Observability
Goal: we can see what's happening in production.

- [ ] Structured logs everywhere with request IDs / submission IDs as fields
- [ ] Prometheus metrics: HTTP latency, queue depth, judge duration, verdict distribution
- [ ] `/metrics` endpoint
- [ ] Decision later: tracing (OpenTelemetry)

### Phase 13 — Stretch (post-MVP)
- [ ] Contests (start/end time, problem set, scoring)
- [ ] Leaderboards (Redis sorted sets)
- [ ] More languages (C++, Java, Rust)
- [ ] Replace Docker with `nsjail` or `isolate` for faster sandbox startup
- [ ] Frontend (separate project)

---

## Decisions log

Record non-obvious choices here so future-us / agents understand the why.

- **2026-05-01** — Project initialized. Chose Go + Postgres + Redis Streams + Docker sandbox as the v1 stack. Rationale in chat history; key driver: production-credible patterns with the smallest viable infra footprint.
- **2026-07-06** — Phase 5 done. DB row is the source of truth for a submission; the queue is best-effort. `POST /submissions` persists `status=queued` then enqueues, but never fails the request on an enqueue error — a DB-backed sweeper re-enqueues any `queued` row older than `SWEEP_STALE_SEC`. `queue.Queue.Nack` is a no-op for now (message stays in the consumer-group PEL for later XAUTOCLAIM, added with the worker in Phase 6). Also fixed a bug in `storage/redis` that closed the client immediately after creating it.

## Open questions

- Module path for `go mod init`? Default proposal: `github.com/harshit-mangtani/judge`.
- Project name — keeping `judge` as folder name; rename if a better one comes up.

## How to use this file

1. Read top-to-bottom to get oriented.
2. Find the first phase with unchecked items — that's where work resumes.
3. Tick a box only when the item is **actually working**, not just written.
4. Add new items as we discover them; don't silently change scope.
5. When in doubt, finish the current phase before starting the next.
