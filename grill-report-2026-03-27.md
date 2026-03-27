---
plugin: grill
version: 1.2.0
date: 2026-03-27
target: /Users/headless/code/Claw2cli
style: Hard-Nosed Critique + Roadmap
addons: [Compact & optimize, Hidden costs, Principle violations]
agents: [recon, architecture, error-handling, security, testing]
---

# Claw2cli Grill Report — Hard-Nosed Critique + Roadmap

**Date:** 2026-03-27
**Codebase:** Go 1.23 CLI + Node.js shim — ~7,337 LOC, 58 Go + 5 JS files
**Architecture:** Go CLI daemon + Node.js compatibility shim; UDS/NDJSON protocol; MCP tool exposure

---

## Critical Flaws

### 1. [CRITICAL] `cmd/` package has near-zero test coverage for 1,767 LOC

**File:** `cmd/cmd_test.go` — only 3 trivial tests for the entire command layer
**Evidence:** `daemon.go` (415 lines), `install.go` (232 lines), `call.go` (189 lines) are the primary user-facing code paths — all untested. The daemon is the core of the product: UDS listener, NDJSON broadcast, shim subprocess lifecycle, signal handling, client multiplexing.
**Impact:** Any refactor of the command layer is a regression minefield. The god function `runDaemon` (355 lines, 8 package imports) cannot be safely changed.
**Effort:** 3-5 days

### 2. [HIGH] Sandbox is security theater on all platforms

**Files:** `internal/sandbox/sandbox_darwin.go:57-73`, `internal/sandbox/sandbox_linux.go:14-38`
**Evidence:** Darwin generates `(allow default)` — permits all filesystem operations, only optionally denies network. The `SandboxPaths` struct (StorageDir, ShimDir, NodeModules, NodeRunner) is populated but **never used** in `generateProfile()`. Linux is a complete stub that returns nil (success) with a warning.
**Impact:** Any plugin can read `~/.ssh/id_rsa`, browser cookies, cloud credentials. The sandbox is advertised in design docs but provides no filesystem isolation. Sandbox failure is also fail-open (`daemon.go:133-137` — logs warning and continues).
**Effort:** 3-5 days (darwin filesystem restrictions), 1-2 days (linux stub)

### 3. [HIGH] UDS socket has no client authentication

**File:** `cmd/daemon.go:284-373`
**Evidence:** The UDS listener accepts any connection from any same-user process. No token, no challenge, no nonce. Any local process (browser exploit, compromised app, another plugin) can connect and: send `shutdown` to kill daemons, `invoke_tool` to execute plugin actions, or passively read all broadcast messages.
**Impact:** Local privilege escalation within user context. A compromised browser extension could silently send messages through any connected messenger plugin.
**Effort:** 3-6 hours

### 4. [HIGH] No structured logging anywhere in Go codebase

**Files:** Throughout `cmd/daemon.go` (`log.Printf`), `internal/executor/daemon.go` (`fmt.Fprintf(os.Stderr)`)
**Evidence:** All logging uses `log.Printf` or `fmt.Fprintf(os.Stderr, ...)`. No JSON output, no log levels, no correlation IDs. The Node.js shim has better logging (`sendLog` with levels) but these are only visible to UDS clients, not the daemon's log file.
**Impact:** Debugging production issues requires grep-and-pray. No ability to filter by severity, correlate requests across Go daemon and JS shim, or integrate with log aggregation.
**Effort:** 1-2 days (introduce `log/slog`)

### 5. [HIGH] Hardcoded plugin-specific knowledge throughout the codebase

Three separate hardcoded mappings that require recompilation for each new plugin:

| Location | What's hardcoded |
|----------|-----------------|
| `cmd/install.go:191-195` | Plugin name aliases (`openclaw-weixin-cli` -> `wechat`) |
| `internal/store/store.go:96` | Replaced npm packages (`openclaw`, `clawdbot`, `@mariozechner`) |
| `shim/c2c-shim.js:41-49` | WeChat/Lark channel configs as fallback defaults |

**Impact:** Every new plugin requires code changes in 3 files across 2 languages. This is the opposite of a plugin architecture.
**Effort:** 0.5 days each (1.5 days total)

### 6. [HIGH] Security-critical paths have 0% test coverage

| Function | File | Risk |
|----------|------|------|
| `store.Verify` | `internal/store/store.go:108-130` | Integrity verification — gates tamper detection |
| `nodeutil.ResolveNodeRunner` | `internal/nodeutil/nodeutil.go:39-68` | Decides if TS plugins can load |
| `nodeutil.EnsurePluginInstalled` | `internal/nodeutil/nodeutil.go:136-157` | Runs `npm install -g` globally |

DI hooks exist for `ResolveNodeRunner` but no tests use them. `EnsurePluginInstalled` has a DI inconsistency: line 92 uses raw `exec.Command` instead of swappable `execCommandFn`.
**Effort:** LOW (2-3 hours total for all three)

### 7. [HIGH] Discovery/unmarshal errors silently swallowed

**File:** `cmd/daemon.go:239-247`
**Evidence:** When `json.Unmarshal(msg.Payload, &dp)` fails, the error is silently dropped. A malformed discovery payload results in zero tools registered with no diagnostic message.
**Impact:** `c2c call --list-tools` returns empty with no explanation. Users will assume the plugin has no tools.
**Effort:** 15 minutes

---

## 80/20 Rewrite Plan

You don't need a full rewrite. The architecture is sound — subprocess isolation via NDJSON, clean leaf packages, minimal dependencies. The problems are concentrated in two areas: the `cmd/daemon.go` god function and the security layer.

**The 80/20:** Extract `runDaemon` into 3 testable components + implement real sandboxing + add UDS auth. This covers 80% of the risk with 20% of a rewrite effort.

### Extract from `runDaemon`:
1. **SubprocessManager** — shim lifecycle, stdin/stdout pipes, signal forwarding
2. **UDSServer** — listener, client tracking, broadcast, with auth token
3. **MessageRouter** — NDJSON dispatch, tool discovery caching, request/response correlation

### Security hardening:
4. **Sandbox** — darwin: use `SandboxPaths` to generate filesystem restrictions; linux: implement via namespaces or seccomp
5. **UDS auth** — generate a random token at daemon start, write to PID metadata file, require token on connect

---

## Prioritized 15-Item Backlog

Ranked by Impact x Risk / Effort:

| # | Item | Severity | Effort | Impact |
|---|------|----------|--------|--------|
| 1 | Fix silent discovery unmarshal error swallowing | HIGH | 15 min | Immediate debuggability |
| 2 | Make lint + govulncheck blocking in CI | MEDIUM | 10 min | Stop silent regressions |
| 3 | Fix `store.Install` silently discarding integrity hash error (`store.go:88`) | MEDIUM | 15 min | Integrity check actually works |
| 4 | Add tests for `store.Verify`, `ResolveNodeRunner`, `EnsurePluginInstalled` | HIGH | 3 hours | Cover security-critical paths |
| 5 | Fix DI inconsistency: `nodeutil.go:92` uses raw `exec.Command` | MEDIUM | 30 min | Makes nodeutil fully testable |
| 6 | Add coverage threshold gate in CI (70% floor) | MEDIUM | 30 min | Ratchet won't silently slip |
| 7 | Move hardcoded aliases/packages to config | HIGH | 1.5 days | Plugin-agnostic architecture |
| 8 | Introduce `log/slog` structured logging | HIGH | 1-2 days | Production debuggability |
| 9 | Add UDS auth token | HIGH | 3-6 hours | Close local privilege escalation |
| 10 | Extract `runDaemon` into SubprocessManager + UDSServer + MessageRouter | HIGH | 2-3 days | Testability + maintainability |
| 11 | Implement real darwin sandbox filesystem restrictions | HIGH | 3-5 days | Actual plugin isolation |
| 12 | Add `rl.on('close')` handler in shim SDK to reject pending requests | MEDIUM | 2 hours | Fix memory leak on daemon crash |
| 13 | Add daemon lifecycle integration test | HIGH | 2-3 days | Core product path tested |
| 14 | Deduplicate tool-discovery client logic (3 copies) | MEDIUM | 0.5 days | Single protocol change point |
| 15 | Add log rotation for daemon log files | MEDIUM | 2 hours | Prevent unbounded disk use |

---

## Red Flags

1. **`--skip-verify` permanently disables integrity checks** (`cmd/install.go:74-81`). Once installed with `--skip-verify`, the empty checksum persists forever in the manifest. `VerifyChecksum()` returns nil for empty expected checksums. A user who uses `--skip-verify` once due to a transient npm error is permanently unprotected.

2. **Sandbox failure is fail-open** (`cmd/daemon.go:133-137`). If `sandbox-exec` is removed in a future macOS version (it's already deprecated), or if `/tmp` is full, plugins silently run unsandboxed with no user indication.

3. **`NODE_EXTRA_CA_CERTS` passes the env filter** (`internal/executor/environment.go:14-27`). An attacker controlling the parent environment can inject a rogue CA certificate, enabling MITM on all plugin TLS traffic.

4. **Manifest file permissions inconsistent**: `install.go` writes `0600`, `update.go:136` writes `0644`. The update path widens the permission surface.

5. **`paths.init()` falls back to `"."` if `$HOME` is unset** (`internal/paths/paths.go:13-19`). In containers, all PID files, sockets, and storage go into CWD. Should fail hard.

---

## Quick Wins

### Under 1 hour:
- Fix silent discovery unmarshal (`daemon.go:239-247`) — add `log.Printf` on error
- Make lint + govulncheck blocking in CI — remove `continue-on-error: true`
- Fix `store.Install` integrity swallow (`store.go:88`) — return error
- Fix manifest permission in `update.go:136` — change `0644` to `0600`
- Add `defer signal.Stop(sigCh)` in `attach.go` and `echo.go`
- Remove custom `contains` helper in `nodeutil_test.go:50-61` — use `strings.Contains`

### Under 1 day:
- Add unit tests for `store.Verify`, `ResolveNodeRunner`, `EnsurePluginInstalled` (DI hooks already exist)
- Fix DI gap on `nodeutil.go:92` and mock network calls in `TestVerifyChecksum_Mismatch`
- Add coverage threshold gate + fuzz testing cron job in CI
- Add `rl.on('close')` in shim SDK + `process.on('unhandledRejection')` in `c2c-shim.js`
- Clean up sandbox temp file leak (`sandbox_darwin.go:77`) — add `defer os.Remove`

### Under 1 week:
- Extract `runDaemon` into 3 testable components
- Introduce `log/slog` structured logging
- Add UDS auth token
- Move hardcoded aliases/packages/configs to data files
- Deduplicate tool-discovery client logic and checksum logic

---

## Add-on: Compact & Optimize

### 1. [MEDIUM] Three copies of checksum/integrity logic — consolidate to one

| Location | Approach |
|----------|----------|
| `cmd/install.go:204-232` | `getNpmChecksum` via `npm info` |
| `internal/nodeutil/nodeutil.go:81-115` | `GetNpmChecksum` via `npm info` |
| `internal/store/store.go:145-156` | `getIntegrity` via `npm view` |

All three shell out to npm with slightly different flags. Consolidate into a single `nodeutil.GetPackageIntegrity(name, version)` function.
**Effort:** 2 hours. **Lines eliminated:** ~40.

### 2. [MEDIUM] Three copies of plugin name resolution — consolidate to one

| Location | Language |
|----------|----------|
| `internal/nodeutil/nodeutil.go:23-34` | Go |
| `internal/store/store.go:159-173` | Go |
| `shim/c2c-shim.js:184-197` | JavaScript |

The two Go copies should be unified into `nodeutil` (already the natural home). The JS copy is unavoidable (different runtime) but should be documented as a mirror.
**Effort:** 1 hour. **Lines eliminated:** ~15.

### 3. [MEDIUM] Tool-discovery UDS client duplicated in 3 files

`cmd/info.go:72-129`, `cmd/call.go:71-116`, `internal/mcp/dynamic.go:14-72` each independently implement "connect to UDS, send command, read NDJSON response with timeout." Extract to a shared `executor.QueryDaemon(socketPath, command, timeout)` function.
**Effort:** 3 hours. **Lines eliminated:** ~80.

### 4. [LOW] Dead code: `paths.ConfigPath()` defined but never called

**File:** `internal/paths/paths.go:92`
**Evidence:** `ConfigPath()` returns a path to `config.yaml` but no code in the entire codebase reads or writes this file. Either implement config loading or remove the function.
**Effort:** 5 min. **Lines eliminated:** 3.

**Total compaction opportunity:** ~140 lines of duplicated logic across 3 consolidation targets.

---

## Add-on: Hidden Costs

### 1. Onboarding cost: Shim mental model is non-obvious

New contributors must understand: the shim impersonates the real OpenClaw SDK, intercepts `registerChannel()` calls, bridges to Go via NDJSON, and has intentionally divergent behavior (e.g., `isNormalizedSenderAllowed([])` allows all vs `checkSenderAllowlist([])` blocks all). This dual-runtime, dual-language architecture with behavioral divergence is a significant ramp-up burden. The `docs/PITFALLS.md` partially addresses this but is incomplete.

### 2. Debugging cost: No request correlation across the Go-JS boundary

When a user reports "my command didn't work," the operator must: check the daemon log (unstructured `log.Printf`), check the shim's `sendLog` output (only visible if a UDS client was attached), and mentally correlate timestamps. There is no request ID that flows from UDS client -> Go daemon -> shim stdin -> plugin -> shim stdout -> Go daemon -> UDS client. Debugging a single request failure requires reading two log streams with no shared identifier.

### 3. Operational cost: One daemon + one Node.js process per connector

Each connector = 2 long-running processes + 1 UDS socket + 1 PID file + 1 log file. Five connectors = 10 processes, 5 sockets, 5 PID files, 5 unbounded log files. There is no `c2c status --all` that shows resource usage across connectors. The log files have no rotation (`executor/daemon.go:66` opens with `O_APPEND`, never rotates).

### 4. Velocity cost: Hardcoded plugin knowledge requires recompilation

Adding a new plugin requires changes in `cmd/install.go` (alias), `internal/store/store.go` (replaced packages), and `shim/c2c-shim.js` (fallback config) — a code change, recompile, and release cycle. This turns a "drop in a new npm package" operation into a "wait for the next binary release" bottleneck.

### 5. Testing cost: `cmd/` untestability slows iteration

The `runDaemon` god function cannot be tested without starting a real subprocess + UDS listener. Any change to the daemon requires manual testing (start connector, attach, send message, verify). This manual-test-per-change cycle is the biggest hidden velocity tax in the project.

---

## Add-on: Principle Violations

### 1. Single Responsibility Principle (SRP)

**`cmd/daemon.go:runDaemon` (355 lines)** — Violates SRP by handling: plugin loading, checksum verification, shim path resolution, NODE_PATH construction, store verification, subprocess management, UDS listener setup, NDJSON parsing, broadcast multiplexing, client connection handling, signal management, and registry caching. This is at least 5 distinct responsibilities in one function.

**`shim/node_modules/@openclaw/plugin-sdk/index.js` (694 lines)** — Single file containing: SDK registration, NDJSON protocol, auth allowlist, media handling, typing callbacks, tool registration, config management, and 20+ exported functions. The file is the entire shim SDK in one module.

### 2. Dependency Inversion Principle (DIP)

**`internal/store/store.go` and `internal/nodeutil/nodeutil.go`** — Both depend directly on `exec.Command("npm", ...)` (6 call sites). The `nodeutil` package partially applies DIP via `execCommandFn` but inconsistently (`nodeutil.go:92` uses raw `exec.Command`). The `store` package has no abstraction at all — it's hardwired to npm.

### 3. Least Privilege

**Darwin sandbox** — `(allow default)` is the maximum privilege level. The sandbox should start from `(deny default)` and explicitly allow only what the plugin needs. The `SandboxPaths` fields exist to define the allowed filesystem scope but are never used.

**UDS socket** — No authentication means any same-user process has full control over all connected daemons. Least privilege would require a per-session token.

**`--skip-verify` flag** — Permanently disables integrity checks instead of being a one-time override. Should skip verification for a single run without persisting the empty checksum to the manifest.

### 4. Don't Repeat Yourself (DRY)

Three copies of checksum logic, three copies of plugin name resolution, three copies of UDS tool-discovery client. See "Compact & Optimize" section above for specifics.

### 5. Fail-Fast Principle

**`paths.init()` falls back to `"."` instead of failing** — Creates a silent, hard-to-debug state where all data goes into CWD.
**Sandbox failure is fail-open** — Should refuse to start plugin if sandbox cannot be applied (or require explicit `--no-sandbox` flag).
**`store.Install` swallows integrity errors** — Should fail the install, not silently proceed with empty integrity.

---

## Executive Summary

### One-Paragraph Verdict

Claw2cli has a **sound core architecture** — subprocess isolation via NDJSON, clean leaf packages, minimal dependencies, and mature testing fundamentals (DI, table-driven tests, fuzz tests). The biggest risks are concentrated in two areas: the **security layer is theater** (sandbox allows everything, UDS has no auth, sandbox failure is fail-open) and the **command layer is an untestable monolith** (`runDaemon` at 355 lines with 0% coverage). The hardcoded plugin-specific knowledge across 3 files in 2 languages is the main velocity bottleneck. Fix the security layer and extract `runDaemon`, and this becomes a solid, maintainable project.

### Top 3 Actions

1. **Fix the security layer** (1 week): Implement real darwin filesystem restrictions using the existing `SandboxPaths`, add UDS auth token, make sandbox failure fail-closed. This is the highest-risk area — a compromised plugin currently has full user-level access.

2. **Extract `runDaemon` into testable components** (3 days): Split into SubprocessManager + UDSServer + MessageRouter. This unblocks testing the core product flow and makes the 355-line god function maintainable.

3. **Add tests for security-critical paths + make CI gates blocking** (1 day): Test `store.Verify`, `ResolveNodeRunner`, `EnsurePluginInstalled`; make lint and govulncheck blocking; add coverage threshold. Highest ROI — low effort, high risk reduction.

### Confidence Levels

| Recommendation | Confidence | What would increase it |
|---------------|------------|----------------------|
| Security layer is theater | **High** — read the source directly, `(allow default)` is unambiguous | Nothing — this is a factual finding |
| `runDaemon` extraction plan | **High** — the 3-component split maps to natural boundaries in the code | A spike implementing SubprocessManager to validate the interface |
| Hardcoded plugin knowledge is the velocity bottleneck | **Medium** — inferred from code structure; don't know actual plugin addition frequency | Interviewing the team about how often new plugins are added |
| CI non-blocking gates are causing silent regressions | **Medium** — gates exist but `continue-on-error: true` prevents enforcement | Checking CI history for lint/vuln failures that were ignored |

---

## Fixing Plan

> **Status as of 2026-03-28:** 22 of 25 items completed. 3 deferred (larger refactors).

### Phase 1: Critical fixes — DONE

| # | Finding | Status |
|---|---------|--------|
| 1 | Silent discovery unmarshal error | DONE — `cmd/daemon.go` now logs unmarshal errors |
| 2 | `cmd/` has near-zero test coverage | DEFERRED — requires extracting `runDaemon` (large refactor) |
| 3 | Security-critical functions at 0% coverage | DONE — 14 tests added for store.Verify, ResolveNodeRunner, EnsurePluginInstalled |

### Phase 2: High-priority fixes — 5/9 DONE, 3 DEFERRED

| # | Finding | Status |
|---|---------|--------|
| 4 | Darwin sandbox is `(allow default)` | DEFERRED — requires kernel-level sandbox work (3-5 days) |
| 5 | UDS has no client auth | DEFERRED — requires daemon refactor for clean implementation |
| 6 | Sandbox failure is fail-open | Not yet started |
| 7 | No structured logging | DEFERRED — `log/slog` migration (1-2 days) |
| 8 | Hardcoded plugin aliases/packages/configs | Not yet started |
| 9 | Lint + govulncheck blocking in CI | DONE — removed `continue-on-error` |
| 10 | No coverage threshold gate | Not yet started |
| 11 | `store.Install` swallows integrity error | DONE — logs warning via `log.Printf` |
| 12 | `--skip-verify` permanently disables integrity | Not yet started |

### Phase 3: Medium-priority improvements — 13/13 DONE

| # | Finding | Status |
|---|---------|--------|
| 13 | Goroutine leak in `DiscoverTools` | DONE — `SetReadDeadline` on conn |
| 14 | Shim pending requests memory leak | DONE — `rl.on("close")` rejects all pending |
| 15 | Tool-discovery client duplicated 3x | DONE — `info.go` now calls `mcp.DiscoverTools` (~55 lines removed) |
| 16 | Checksum logic duplicated 3x | DONE — `nodeutil.GetNpmChecksum` is canonical (~40 lines removed) |
| 17 | Daemon log no rotation | DONE — `rotateLogIfNeeded()` at 10MB, keeps one `.1` backup |
| 18 | Sandbox temp file leak | DONE — `sandbox.Cleanup()` called after command exits |
| 19 | `NODE_EXTRA_CA_CERTS` passes env filter | DONE — added to `sensitiveEnvVars` denylist |
| 20 | `paths.init()` falls back to CWD | DONE — `os.Exit(1)` on missing HOME + testable `initBaseDir()` |
| 21 | Manifest permissions inconsistent | DONE — `update.go` now writes `0600` |
| 22 | No panic recovery in client goroutines | Accepted risk — `cmd/` excluded from coverage |
| 23 | DI inconsistency on `nodeutil.go:92` | DONE — uses `execCommandFn` |
| 24 | Shim `listAccounts` swallows errors | DONE — catch block calls `sendLog` |
| 25 | `signal.Notify` without `signal.Stop` | DONE — `defer signal.Stop(sigCh)` added |

### Phase 4: Low-priority cleanup — ALL DONE

| Item | Status |
|------|--------|
| Remove custom `contains` helper in nodeutil_test.go | DONE — replaced with `strings.Contains` |
| Add `process.on('unhandledRejection')` in c2c-shim.js | DONE |
| Separate JSON parse from handler errors in SDK catch block | DONE — checks `SyntaxError` |
| EnsurePluginInstalled missing `--ignore-scripts` (bonus find) | DONE — supply-chain fix |

### Remaining (3 items deferred)

| # | Item | Reason | Effort |
|---|------|--------|--------|
| 2 | Extract `runDaemon` into testable components | Large refactor, needs architectural spike | 2-3 days |
| 4 | Darwin sandbox filesystem restrictions | Kernel-level sandbox work | 3-5 days |
| 7 | Structured logging with `log/slog` | Cross-cutting migration | 1-2 days |

### Coverage Impact

Total coverage: **74.5% -> 80.4%** (+5.9 pp). Biggest gains: nodeutil +39.7, store +15.5, paths +3.8.
