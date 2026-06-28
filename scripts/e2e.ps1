<#
.SYNOPSIS
    Full end-to-end check of the GoSpoc judge platform (phases 0-9).

.DESCRIPTION
    Brings up Postgres + Redis, migrates, builds and starts the API and worker,
    then exercises every phase: HTTP middleware, auth/roles, problems, submission
    intake + validation, the Redis queue, and real judging in Python and Go
    across all verdicts (AC/WA/TLE/MLE/RE/CE). Prints a PASS/FAIL summary.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\e2e.ps1

.PARAMETER SkipInfra
    Assume Postgres/Redis are already up and migrated.
.PARAMETER TearDown
    Run `docker compose down` at the end (default: leave containers running).
#>
[CmdletBinding()]
param(
    [string]$BaseUrl = "http://localhost:8080",
    [switch]$SkipInfra,
    [switch]$TearDown
)

# Native tools write progress to stderr; we judge each step on its own merits.
$ErrorActionPreference = "Continue"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$script:pass = 0
$script:fail = 0
function Section($t) { Write-Host ""; Write-Host "== $t ==" -ForegroundColor Cyan }
function Info($t)    { Write-Host "   $t" -ForegroundColor DarkGray }
function Check($name, [bool]$ok, $detail = "") {
    if ($ok) { Write-Host ("   [PASS] {0}" -f $name) -ForegroundColor Green; $script:pass++ }
    else     { Write-Host ("   [FAIL] {0}" -f $name) -ForegroundColor Red; if ($detail) { Write-Host "          $detail" -ForegroundColor Red }; $script:fail++ }
}

# curl.exe wrapper. Body goes through a temp file (--data-binary @file) because
# inline quoted JSON gets mangled by Windows argument parsing.
function Api {
    param([string]$Method, [string]$Path, [string]$Body, [string]$Token)
    $a = @("-s", "-w", "`n%{http_code}", "-X", $Method, "$BaseUrl$Path")
    if ($Token) { $a += @("-H", "Authorization: Bearer $Token") }
    $tmp = $null
    if ($Body) {
        $tmp = [IO.Path]::GetTempFileName()
        [IO.File]::WriteAllText($tmp, $Body, (New-Object Text.UTF8Encoding($false)))
        $a += @("-H", "Content-Type: application/json", "--data-binary", "@$tmp")
    }
    try { $raw = (& curl.exe @a) -join "`n" } finally { if ($tmp) { Remove-Item $tmp -Force -EA SilentlyContinue } }
    $lines = $raw -split "`n"
    return @{ Status = [int]$lines[-1]; Body = ($lines[0..($lines.Length - 2)] -join "`n") }
}
function Header($path, $name) {
    $h = (& curl.exe -s -D - -o NUL "$BaseUrl$path" 2>$null) -join "`n"
    return [bool]($h -match "(?im)^${name}\s*:")
}
function Psql($sql) { docker compose exec -T postgres psql -U judge -d judge -t -A -c $sql }
function Wait-For([scriptblock]$Test, [int]$Retries = 40, [int]$DelayMs = 500) {
    for ($i = 0; $i -lt $Retries; $i++) { try { if (& $Test) { return $true } } catch {}; Start-Sleep -Milliseconds $DelayMs }
    return $false
}
function WaitVerdict($sid, $token, $retries = 80) {
    for ($i = 0; $i -lt $retries; $i++) {
        Start-Sleep -Milliseconds 750
        $j = (Api GET "/submissions/$sid" $null $token).Body | ConvertFrom-Json
        if ($j.status -eq "done" -or $j.status -eq "failed") { return $j }
    }
    return $null
}

Write-Host "GoSpoc end-to-end check (phases 0-9)" -ForegroundColor White
$apiProc = $null; $workerProc = $null
$compose = @("compose", "-f", "compose.yaml")

try {
    # --- Phase 0: toolchain -------------------------------------------------
    Section "Preflight"
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        foreach ($p in @((Join-Path $env:ProgramFiles "Go\bin"), (Join-Path $env:LOCALAPPDATA "Programs\Go\bin"), (Join-Path $env:USERPROFILE "go\bin"), "C:\Go\bin")) {
            if ($p -and (Test-Path (Join-Path $p "go.exe"))) { $env:PATH = "$p;$env:PATH"; Info "added Go to PATH: $p"; break }
        }
    }
    Check "go on PATH" ([bool](Get-Command go -EA SilentlyContinue))
    $dockerOk = $false; try { docker version --format '{{.Server.Version}}' | Out-Null; $dockerOk = $? } catch {}
    Check "docker daemon reachable" $dockerOk
    if (-not $dockerOk) { throw "Start Docker Desktop and retry." }

    # --- Phase 2/5: infra ---------------------------------------------------
    if (-not $SkipInfra) {
        Section "Infra (Postgres + Redis + migrations)"
        & docker @compose up -d 2>&1 | Out-Null
        Check "postgres accepting connections" (Wait-For { (& docker @compose exec -T postgres pg_isready -U judge) -match "accepting connections" })
        Check "redis responding to PING" (Wait-For { (& docker @compose exec -T redis redis-cli ping) -match "PONG" })
        $dbUrl = "postgres://judge:judge@host.docker.internal:5432/judge?sslmode=disable"
        $mig = & docker run --rm -v "${repoRoot}/migrations:/migrations" migrate/migrate -path=/migrations -database $dbUrl up 2>&1 | Out-String
        Check "migrations applied" (($LASTEXITCODE -eq 0) -or ($mig -match "no change")) $mig.Trim()
    }

    Section "Sandbox images"
    foreach ($img in @(@("gospoc-sandbox-python", "sandbox/Dockerfile.python"), @("gospoc-sandbox-go", "sandbox/Dockerfile.golang"))) {
        docker image inspect $img[0] *> $null
        if ($LASTEXITCODE -ne 0) { Info "building $($img[0]) (first run, a few minutes)..."; & docker build -f $img[1] -t $img[0] . *> $null }
        docker image inspect $img[0] *> $null
        Check "$($img[0]) image present" ($LASTEXITCODE -eq 0)
    }

    # --- Phase 0/1/6: build + start API -------------------------------------
    Section "Build"
    New-Item -ItemType Directory -Force -Path "$repoRoot\bin" | Out-Null
    & go build -o "bin\api.exe" ./cmd/api;    Check "go build ./cmd/api" ($LASTEXITCODE -eq 0)
    & go build -o "bin\worker.exe" ./cmd/worker; Check "go build ./cmd/worker" ($LASTEXITCODE -eq 0)

    $apiProc = Start-Process "$repoRoot\bin\api.exe" -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput "$repoRoot\bin\api.e2e.log" -RedirectStandardError "$repoRoot\bin\api.e2e.err.log"
    if (-not (Wait-For { (Api GET "/healthz").Status -eq 200 })) { throw "API did not become healthy" }

    # --- Phase 1: HTTP middleware ------------------------------------------
    Section "Phase 1 - HTTP server + middleware"
    Check "GET /healthz -> 200" ((Api GET "/healthz").Status -eq 200)
    Check "response carries X-Request-ID header" (Header "/healthz" "X-Request-ID")
    Check "panic recovery: GET /panic -> 500 (server survives)" ((Api GET "/panic").Status -eq 500)
    Check "server still alive after panic" ((Api GET "/healthz").Status -eq 200)

    # --- Phase 2: auth + users ---------------------------------------------
    Section "Phase 2 - Auth + users"
    $stamp = Get-Random
    $admin = "admin$stamp@example.com"; $user = "user$stamp@example.com"; $pw = "supersecret123"
    Check "signup admin -> 201" ((Api POST "/auth/signup" (@{email=$admin;password=$pw;role="admin"} | ConvertTo-Json) $null).Status -eq 201)
    Check "signup user -> 201"  ((Api POST "/auth/signup" (@{email=$user;password=$pw} | ConvertTo-Json) $null).Status -eq 201)
    $adminLogin = Api POST "/auth/login" (@{email=$admin;password=$pw} | ConvertTo-Json) $null
    $adminTok = ($adminLogin.Body | ConvertFrom-Json).token
    Check "login admin -> 200 + token" ($adminLogin.Status -eq 200 -and $adminTok)
    $userTok = (Api POST "/auth/login" (@{email=$user;password=$pw} | ConvertTo-Json) $null).Body | ConvertFrom-Json | Select-Object -Expand token
    Check "login wrong password -> 401" ((Api POST "/auth/login" (@{email=$admin;password="wrongpass1"} | ConvertTo-Json) $null).Status -eq 401)
    Check "GET /me (auth) -> 200" ((Api GET "/me" $null $adminTok).Status -eq 200)
    Check "GET /me (no token) -> 401" ((Api GET "/me").Status -eq 401)

    # --- Phase 3: problems --------------------------------------------------
    Section "Phase 3 - Problems"
    $slug = "sum-$stamp"
    $create = Api POST "/problems" (@{slug=$slug;title="Sum";statement="print a+b";time_limit_ms=2000;memory_limit_kb=262144} | ConvertTo-Json) $adminTok
    $probId = ($create.Body | ConvertFrom-Json).id
    Check "POST /problems (admin) -> 201" ($create.Status -eq 201 -and $probId)
    Check "POST /problems (regular user) -> 403" ((Api POST "/problems" (@{slug="x$stamp";title="x";statement="x";time_limit_ms=1000;memory_limit_kb=1000} | ConvertTo-Json) $userTok).Status -eq 403)
    Check "GET /problems -> 200" ((Api GET "/problems" $null $adminTok).Status -eq 200)
    Check "GET /problems/{slug} -> 200" ((Api GET "/problems/$slug" $null $adminTok).Status -eq 200)
    # low-memory problem for the MLE case
    $slugM = "mle-$stamp"
    $probMle = ((Api POST "/problems" (@{slug=$slugM;title="Mle";statement="alloc";time_limit_ms=3000;memory_limit_kb=131072} | ConvertTo-Json) $adminTok).Body | ConvertFrom-Json).id
    $i0 = 400000 + (Get-Random -Max 90000)
    Psql "INSERT INTO test_cases (problem_id, idx, input, expected_output, is_sample) VALUES ($probId,$i0,'2 3','5',true),($probId,$($i0+1),'10 20','30',false),($probMle,$($i0+2),'x','0',false);" | Out-Null
    Check "seed test cases" ($LASTEXITCODE -eq 0)

    # --- Phase 4: submission intake ----------------------------------------
    Section "Phase 4 - Submission intake + validation"
    $firstSub = Api POST "/submissions" (@{problem_id=$probId;language="python";source="print(1)"} | ConvertTo-Json) $adminTok
    $firstId = ($firstSub.Body | ConvertFrom-Json).id
    Check "POST /submissions -> 202 + id" ($firstSub.Status -eq 202 -and $firstId)
    Check "GET /submissions/{id} -> status queued" (((Api GET "/submissions/$firstId" $null $adminTok).Body | ConvertFrom-Json).status -eq "queued")
    Check "unsupported language -> 400" ((Api POST "/submissions" (@{problem_id=$probId;language="cobol";source="x"} | ConvertTo-Json) $adminTok).Status -eq 400)
    Check "missing problem_id -> 400" ((Api POST "/submissions" (@{language="python";source="x"} | ConvertTo-Json) $adminTok).Status -eq 400)
    Check "other user GET submission -> 403" ((Api GET "/submissions/$firstId" $null $userTok).Status -eq 403)
    Check "GET /submissions?problem_id -> 200" ((Api GET "/submissions?problem_id=$probId" $null $adminTok).Status -eq 200)

    # --- Phase 5: queue -----------------------------------------------------
    Section "Phase 5 - Redis queue"
    $before = [int]((& docker @compose exec -T redis redis-cli XLEN submissions) | Select-Object -First 1)
    Api POST "/submissions" (@{problem_id=$probId;language="python";source="print(1)"} | ConvertTo-Json) $adminTok | Out-Null
    $after = [int]((& docker @compose exec -T redis redis-cli XLEN submissions) | Select-Object -First 1)
    Check "submission enqueued on stream (XLEN grew)" ($after -gt $before) "before=$before after=$after"

    # --- Phases 6-9: worker + real judging ---------------------------------
    Section "Phases 6-9 - Worker + judging (all verdicts)"
    $pyAC  = "a,b=map(int,input().split())`nprint(a+b)"
    $pyWA  = "a,b=map(int,input().split())`nprint(a-b)"
    $pyTLE = "while True:`n    pass"
    $pyRE  = "import sys`nsys.exit(1)"
    $pyMLE = "x=bytearray(200*1024*1024)`nprint(len(x))"
    $goAC  = "package main`nimport `"fmt`"`nfunc main(){var a,b int;fmt.Scan(&a,&b);fmt.Println(a+b)}"
    $goCE  = "package main`nfunc main(){ totallyUndefined() }"

    function Sub($pblm, $lang, $src) { return ((Api POST "/submissions" (@{problem_id=$pblm;language=$lang;source=$src} | ConvertTo-Json) $adminTok).Body | ConvertFrom-Json).id }
    $jobs = @(
        @{ name="python AC";  id=(Sub $probId "python" $pyAC);  want="AC" },
        @{ name="python WA";  id=(Sub $probId "python" $pyWA);  want="WA" },
        @{ name="python TLE"; id=(Sub $probId "python" $pyTLE); want="TLE" },
        @{ name="python RE";  id=(Sub $probId "python" $pyRE);  want="RE" },
        @{ name="python MLE"; id=(Sub $probMle "python" $pyMLE); want="MLE" },
        @{ name="go AC";      id=(Sub $probId "go" $goAC);      want="AC" },
        @{ name="go CE";      id=(Sub $probId "go" $goCE);      want="CE" }
    )
    Info "submitted $($jobs.Count) solutions; starting worker..."
    $workerProc = Start-Process "$repoRoot\bin\worker.exe" -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput "$repoRoot\bin\worker.e2e.log" -RedirectStandardError "$repoRoot\bin\worker.e2e.err.log"

    foreach ($j in $jobs) {
        $res = WaitVerdict $j.id $adminTok
        if ($res) { Check ("{0} -> {1}" -f $j.name, $j.want) ($res.verdict -eq $j.want) ("got verdict=$($res.verdict) status=$($res.status)") }
        else      { Check ("{0} -> {1}" -f $j.name, $j.want) $false "did not finish" }
    }

    # judging side-effects
    $acId = $jobs[0].id
    $acRows = [int](Psql "SELECT count(*) FROM submission_test_results WHERE submission_id=$acId AND verdict='AC';")
    Check "AC submission wrote per-test rows" ($acRows -ge 2) "AC rows=$acRows"
    $ceErr = (Api GET "/submissions/$($jobs[6].id)" $null $adminTok).Body | ConvertFrom-Json
    Check "CE submission stored compiler output" ([bool]$ceErr.compile_error) "compile_error=$($ceErr.compile_error)"
}
finally {
    Section "Teardown"
    foreach ($p in @($workerProc, $apiProc)) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -EA SilentlyContinue } }
    Info "api + worker stopped."
    if ($TearDown) { & docker @compose down | Out-Null; Info "docker compose down." }
    else { Info "containers left running (stop with: docker compose down)" }
}

Write-Host ""
$total = $script:pass + $script:fail
if ($script:fail -eq 0) { Write-Host ("ALL GREEN: {0}/{1} checks passed" -f $script:pass, $total) -ForegroundColor Green; exit 0 }
else { Write-Host ("{0}/{1} passed, {2} FAILED" -f $script:pass, $total, $script:fail) -ForegroundColor Red; exit 1 }
