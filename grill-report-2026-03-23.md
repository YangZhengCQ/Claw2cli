---
plugin: grill
version: 1.2.0
date: 2026-03-23
target: /Users/headless/code/Claw2cli
style: Hard-Nosed Critique + Roadmap
addons: Compact & optimize, Hidden costs, Assumptions audit
agents: architecture, error-handling, security, testing
---

# Claw2Cli Grill Report

## Hard-Nosed Critique

### Critical Flaws (5)

**C1. Command injection via unsanitized plugin source** `[CRITICAL]`
- **File:** `cmd/install.go:147-153`, `cmd/daemon.go:394-413`
- **Evidence:** `exec.Command("npm", "install", "-g", source)` where `source` comes from CLI args or `manifest.yaml`. While Go's `exec.Command` doesn't invoke a shell, a malicious manifest can point `source` to an arbitrary npm package with pre/post-install hooks that execute arbitrary code. `npx -y` in `runner.go:76` auto-confirms without consent.
- **Impact:** Full code execution as the current user. Supply chain attack vector.
- **Effort:** 1 day

**C2. `cmd/daemon.go` is a 413-line god file** `[CRITICAL]`
- **File:** `cmd/daemon.go`
- **Evidence:** Contains: Cobra command, shim directory resolution, global state (`isForeground`, `toolRegistry`), tool registry accessors, 250-line daemon main loop with 4 goroutines, Node.js/npm helpers, plugin package resolution, plugin installation logic. Impossible to unit test.
- **Impact:** All daemon bugs are untestable; refactoring risk is high.
- **Effort:** 3 days

**C3. `cmd/` package has 0% test coverage (1,492 lines)** `[CRITICAL]`
- **File:** All 14 files in `cmd/`
- **Evidence:** Contains pure functions (`derivePluginName`, `resolvePluginPackage`) and critical orchestration (`runDaemon`) with zero tests. `go test -cover ./cmd/` → 0.0%.
- **Impact:** Regressions ship undetected. The most complex code (daemon lifecycle) is the least tested.
- **Effort:** 3 days for pure functions + extracted logic; daemon integration tests are harder

**C4. Silent npm install failures cause confusing downstream crashes** `[CRITICAL]`
- **File:** `cmd/daemon.go:410` — `install.Run()` return value discarded
- **Evidence:** If `npm install -g` fails (network, permissions, disk), daemon proceeds to launch shim, which fails to import the plugin. User sees "Cannot find module" instead of "npm install failed".
- **Impact:** Debugging a network issue becomes debugging a module resolution error.
- **Effort:** 0.5 day

**C5. UDS socket has no authentication** `[HIGH → treated as CRITICAL for multi-user systems]`
- **File:** `cmd/daemon.go:155-159`, `internal/paths/paths.go:69`
- **Evidence:** Socket created in `~/.c2c/sockets/` (dir mode 0755). No peer credential check. Any local user can connect, read messages (PII), send commands (impersonate the authenticated WeChat user).
- **Impact:** Data exfiltration and impersonation on shared systems.
- **Effort:** 1 day (set socket dir to 0700 + SO_PEERCRED check)

### High-Severity Issues (8)

| # | Finding | File | Effort |
|---|---------|------|--------|
| H1 | Path traversal via plugin name — `../../etc/evil` escapes `~/.c2c/` | `internal/paths/paths.go:35-51` | 0.5d |
| H2 | Checksum recorded but never verified at runtime | `cmd/install.go:83`, `cmd/daemon.go:87-113` | 1d |
| H3 | JSON injection via `fmt.Sprintf` in MCP connector handler | `internal/mcp/server.go:204-206` | 0.5d |
| H4 | Environment variables (AWS keys, tokens) leaked to plugin subprocesses | `internal/executor/environment.go:12-21` | 1d |
| H5 | `resolvePluginPackage` duplicated in Go and JS — divergence = wrong package | `cmd/daemon.go:376-390`, `shim/c2c-shim.js:156-169` | 0.5d |
| H6 | No structured logging — `log.Printf` only, no levels, no correlation | `cmd/daemon.go` (multiple) | 2d |
| H7 | `StopConnector` calls `process.Wait()` on released (non-child) process — may block forever | `internal/executor/daemon.go:123-135` | 0.5d |
| H8 | No CI/CD pipeline — no automated test, lint, or race detection | (missing) | 1d |

### Medium-Severity Issues (10)

| # | Finding | File |
|---|---------|------|
| M1 | Dead `internal/config` package + unused Viper (14 transitive deps) | `internal/config/config.go` |
| M2 | Non-atomic UDS stdin writes — concurrent clients can corrupt NDJSON | `cmd/daemon.go:287-288` |
| M3 | `connect.go` sets global `isForeground` then calls `runDaemon` — untestable coupling | `cmd/connect.go:47` |
| M4 | Log files created with 0644 — other users can read chat messages | `internal/executor/daemon.go:61` |
| M5 | Base directories 0755 instead of 0700 | `internal/paths/paths.go:69` |
| M6 | `DiscoverTools` and `InvokeTool` at 0% coverage | `internal/mcp/dynamic.go` |
| M7 | No `-race` flag in Makefile test targets | `Makefile:12` |
| M8 | Request IDs use `time.Now().UnixNano()` — not unique under concurrency | `internal/mcp/dynamic.go:20` |
| M9 | Daemon crash leaves stale PID/socket files | `cmd/daemon.go:317-320` |
| M10 | `mcp` package at 42.4% coverage | `internal/mcp/` |

---

## 80/20 Rewrite Plan

Don't rewrite. The architecture is sound — 6 clean internal packages, correct dependency direction, no circular imports. The issues are:
1. **Security hardening** (input validation, permissions, checksum verification)
2. **Extract logic from `cmd/daemon.go`** into testable internal packages
3. **Add tests** for extracted code + CI pipeline

### What to keep
- Package structure (`internal/parser`, `internal/protocol`, `internal/executor`, `internal/paths`)
- NDJSON protocol design and message types
- Dependency injection pattern (`deps.go` files)
- Shim architecture (fake SDK approach is correct)
- Capability discovery design (shim translates, daemon caches, consumers query)

### What to extract from `cmd/daemon.go`
- `internal/noderunner/` — `resolveNodeRunner`, `resolveGlobalNodeModules`, `resolvePluginPackage`, `ensurePluginInstalled`
- `internal/registry/` — `toolRegistry`, `GetDiscoveredTools`, `GetAllDiscoveredTools`
- `internal/daemon/` — `runDaemon` core loop (UDS server + subprocess management)

---

## Prioritized 15-Item Backlog

Ranked by Impact × Risk / Effort:

| Rank | Item | Severity | Impact | Effort | Category |
|------|------|----------|--------|--------|----------|
| 1 | Sanitize plugin names (reject `..`, `/`, `\`) | CRITICAL | Supply chain | 0.5d | Security |
| 2 | Set sockets/logs/pids dirs to 0700 | CRITICAL | Data leak | 0.5d | Security |
| 3 | Return error from `ensurePluginInstalled` | CRITICAL | UX | 0.5d | Error handling |
| 4 | Replace `fmt.Sprintf` JSON with `protocol.NewCommand` in MCP | HIGH | Injection | 0.5d | Security |
| 5 | Add `-race` to Makefile + GitHub Actions CI | HIGH | Regressions | 1d | Testing |
| 6 | Extract pure functions from cmd/ → internal/ + add tests | CRITICAL | Testability | 2d | Architecture |
| 7 | Filter env vars passed to plugin subprocesses | HIGH | Credential leak | 1d | Security |
| 8 | Add mutex around `pluginStdin.Write` | MEDIUM | Data corruption | 0.5d | Reliability |
| 9 | Verify checksum before `npx`/`tsx` execution | HIGH | Supply chain | 1d | Security |
| 10 | Remove dead `internal/config` + Viper dependency | MEDIUM | Binary size | 0.5d | Cleanup |
| 11 | Daemon self-cleanup on crash (PID/socket files) | MEDIUM | Stale state | 0.5d | Reliability |
| 12 | Test `DiscoverTools`/`InvokeTool` with `net.Pipe()` | HIGH | Coverage | 1d | Testing |
| 13 | Add structured logging (slog or zerolog) | HIGH | Debuggability | 2d | Observability |
| 14 | Integration test: install → connect → call → stop | MEDIUM | E2E confidence | 2d | Testing |
| 15 | Use `crypto/rand` for request IDs instead of timestamp | MEDIUM | Correctness | 0.5d | Reliability |

---

## Red Flags

1. **No CI means every push is untested.** The TDD Guardian plugin gates commits locally, but there's no server-side enforcement.
2. **Silent `npm install -g` is a supply chain risk.** Both `resolveNodeRunner` and `ensurePluginInstalled` modify the global npm environment without consent.
3. **The shim fake SDK has no version pinning.** When upstream `@openclaw/plugin-sdk` adds new exports, plugins break silently with "function is not a function" errors.

## Quick Wins

### < 1 day
- Sanitize plugin names: `if strings.Contains(name, "..") || strings.Contains(name, "/") { return error }`
- Change `EnsureDirs` to use 0700 for all directories
- Return and check error from `ensurePluginInstalled`
- Replace `fmt.Sprintf` JSON construction with `json.Marshal` in MCP server
- Add `-race` flag to Makefile test target
- Remove `internal/config` and `viper` from `go.mod`

### < 1 week
- Extract pure functions from `cmd/daemon.go` into `internal/` packages
- Add unit tests for extracted functions (resolvePluginPackage, derivePluginName, etc.)
- Set up GitHub Actions CI (go vet, go test -race, coverage threshold)
- Test `DiscoverTools`/`InvokeTool` with net.Pipe() mocks
- Add env var allowlist for plugin subprocesses

---

## Add-on: Compact & Optimize

| What | Where | Action | Savings |
|------|-------|--------|---------|
| Dead `internal/config` package | `internal/config/config.go` + test | Delete entirely | ~80 LOC + 14 transitive deps (Viper) |
| `buildToolDescription` called once, could inline | `shim/.../index.js:152-160` | Already minimal | — |
| `contains`/`containsSub` helpers reimplement `strings.Contains` | `internal/executor/daemon_test.go:421-432` | Replace with `strings.Contains` | 12 LOC |
| `c2c echo` is a debug tool shipped in production binary | `cmd/echo.go` | Move behind build tag or keep (it's useful for integration testing) | — |
| Duplicate `resolvePluginPackage` in Go and JS | `cmd/daemon.go` + `shim/c2c-shim.js` | Single source of truth: shim does resolution, Go passes raw source | ~15 LOC dedup |
| Busy-wait log follow | `cmd/logs.go:41-43` | Replace with fsnotify (already transitive dep via Viper) — but if Viper is removed, not worth adding fsnotify just for this | — |

**Verdict:** Removing `internal/config` + Viper is the biggest win — eliminates dead code and 14 transitive dependencies, shrinking the binary and reducing `go mod tidy` noise.

---

## Add-on: Hidden Costs

| # | Hidden Cost | Impact | Where |
|---|-------------|--------|-------|
| 1 | **Debugging daemon issues requires reading raw NDJSON logs** — no structured logging, no log levels, no request tracing. Every production issue starts with `tail -f ~/.c2c/logs/wechat.log` and grep. | High — operational cost scales with connector count | `cmd/daemon.go` |
| 2 | **Onboarding cost: `cmd/daemon.go` is 413 lines with 4 goroutines** — a new contributor must understand subprocess management, UDS protocol, NDJSON parsing, signal handling, and tool registry all in one file. | High — velocity cost for new contributors | `cmd/daemon.go` |
| 3 | **Silent npm install side-effects** — when `tsx` or a plugin package isn't installed, c2c silently modifies the global npm environment. Users don't know this happened until they notice unexpected global packages. Support tickets. | Medium — trust erosion | `cmd/daemon.go:354-413` |
| 4 | **Shim fake SDK maintenance burden** — every upstream `@openclaw/plugin-sdk` release may add exports that the shim doesn't implement. Discovering this requires a user to report a plugin failure, then manually diffing upstream exports. | High — ongoing maintenance tax | `shim/node_modules/@openclaw/plugin-sdk/index.js` |
| 5 | **No CI means manual regression testing** — every commit requires local `go test`, coverage check, and TDD Guardian gate. This is ~30s overhead per commit, but the real cost is when someone forgets or bypasses it. | Medium — quality drift over time | (missing CI) |

---

## Add-on: Assumptions Audit

| # | Assumption | Evidence | Risk if Wrong | Validation Plan |
|---|-----------|----------|---------------|-----------------|
| 1 | **Node.js and npm are always available at runtime** | Checked at `install` time (`checkNodeNpm`) but NOT at `connect`/`run` time | Crash with confusing error | Add check in `runDaemon` and `RunSkill` |
| 2 | **Plugin npm packages have pre-compiled JS or tsx can handle them** | Verified for WeChat (ESM+TS). Assumption: all OpenClaw plugins follow this pattern | Plugins that need a build step will fail silently | Test with 3+ plugins before beta |
| 3 | **Single-account is the common case** | `handleInvokeTool` auto-picks first account if none specified | Multi-account users get wrong account | Add `accountId` to tool schema as optional param |
| 4 | **contextToken is always cached after first inbound message** | WeChat plugin caches it per (accountId, to) pair in memory | Daemon restart loses all tokens; users must re-send a message | Document this; consider persisting tokens to disk |
| 5 | **`npm root -g` returns the correct global modules path** | Works on Homebrew Node.js (macOS). Untested on nvm, fnm, volta, asdf | Shim fails to find plugin package on alternative Node managers | Test with nvm/volta in CI matrix |
| 6 | **UDS is the right IPC for all platforms** | Unix-only; explicitly no Windows support | Limits adoption to macOS/Linux | Acceptable — documented constraint |
| 7 | **Plugins don't need the real OpenClaw runtime beyond what the shim provides** | Verified for WeChat + Feishu (stubs). Assumption: no plugin uses OpenClaw internals beyond the SDK | New plugins may import deeper OpenClaw modules | Monitor plugin load failures; add stub-on-demand |
| 8 | **Global npm install is acceptable** | `npm install -g` modifies user's global environment | Corporate environments may restrict global installs; CI containers may not have write access | Add `--prefix` option to install to `~/.c2c/node_modules/` instead |

---

## Executive Summary

### One-Paragraph Verdict

Claw2Cli is a well-conceived alpha with sound architecture (clean package boundaries, correct dependency direction, pragmatic shim design) that successfully runs a real WeChat plugin end-to-end. However, it has **serious security gaps** (unsanitized plugin names, unauthenticated UDS, env var leakage), a **413-line untestable god file** (`cmd/daemon.go`), and **zero CI enforcement**. The internal packages are well-tested (91-100%), but the most critical code (daemon lifecycle, MCP dynamic tools) has 0-42% coverage. The biggest risk is that a supply chain attack via a malicious manifest.yaml could achieve code execution with no checksum verification.

### Top 3 Actions

1. **Security hardening (2 days):** Sanitize plugin names, set all dirs to 0700, replace `fmt.Sprintf` JSON with `json.Marshal`, filter env vars, return errors from npm install. This closes the CRITICAL and HIGH security findings.

2. **Extract `cmd/daemon.go` into testable packages (3 days):** Move node runner, tool registry, and plugin resolution into `internal/`. Add unit tests for pure functions. This makes the most complex code testable and unblocks coverage growth.

3. **Set up CI (1 day):** GitHub Actions with `go vet`, `go test -race`, coverage threshold. This prevents regression and enforces quality gates server-side.

### Confidence Level

| Recommendation | Confidence | What would increase it |
|----------------|-----------|----------------------|
| Security hardening | **High** | Penetration test on a multi-user system |
| Extract daemon.go | **High** | N/A — straightforward refactoring |
| CI setup | **High** | N/A — standard practice |
| Remove Viper | **Medium** | Confirm no future config needs before deleting |
| Structured logging | **Medium** | Profile actual debugging sessions to confirm pain |

---

## Fixing Plan

### Phase 1: Critical fixes (do immediately)

1. **Sanitize plugin names**
   - Finding: C1, H1
   - Fix: In `internal/paths/paths.go`, add `ValidateName(name) error` that rejects `..`, `/`, `\`, empty string. Call from `LoadPlugin`, `PluginDir`, `StorageDir`.
   - Effort: 0.5 day
   - Files: `internal/paths/paths.go`, `internal/parser/manifest.go`, `cmd/install.go`

2. **Set directory permissions to 0700**
   - Finding: C5, M5
   - Fix: Change `EnsureDirs` mode from 0755 to 0700 for sockets, pids, plugins dirs. Set log file mode to 0600.
   - Effort: 0.5 day
   - Files: `internal/paths/paths.go`, `internal/executor/daemon.go`

3. **Return error from `ensurePluginInstalled`**
   - Finding: C4
   - Fix: Change signature to `func ensurePluginInstalled(source string) error`, check return value in `runDaemon`.
   - Effort: 0.5 day
   - Files: `cmd/daemon.go`

4. **Replace `fmt.Sprintf` JSON with `json.Marshal`**
   - Finding: H3
   - Fix: Use `protocol.NewCommand` + `json.Marshal` instead of string interpolation.
   - Effort: 0.5 day
   - Files: `internal/mcp/server.go`

### Phase 2: High-priority fixes (this sprint)

5. **Add `-race` flag + GitHub Actions CI**
   - Finding: H8, M7
   - Fix: Update Makefile, add `.github/workflows/ci.yml`.
   - Effort: 1 day
   - Files: `Makefile`, `.github/workflows/ci.yml` (new)

6. **Extract pure functions from `cmd/daemon.go`**
   - Finding: C2, C3
   - Fix: Create `internal/noderunner/`, `internal/registry/`. Move `resolvePluginPackage`, `resolveNodeRunner`, `ensurePluginInstalled`, `GetDiscoveredTools`, `toolRegistry`. Add unit tests.
   - Effort: 3 days
   - Files: `cmd/daemon.go`, new `internal/noderunner/`, `internal/registry/`

7. **Filter env vars for plugin subprocesses**
   - Finding: H4
   - Fix: In `BuildEnv`, construct a clean env with only PATH, HOME, USER, LANG, and C2C_* vars instead of inheriting all.
   - Effort: 1 day
   - Files: `internal/executor/environment.go`

8. **Add mutex for shim stdin writes**
   - Finding: M2
   - Fix: Wrap `pluginStdin` with a mutex-guarded writer.
   - Effort: 0.5 day
   - Files: `cmd/daemon.go`

9. **Runtime checksum verification**
   - Finding: H2
   - Fix: Before `exec.Command("npx"...)` or `exec.Command(nodeRunner...)`, verify the installed package checksum matches manifest.
   - Effort: 1 day
   - Files: `internal/executor/runner.go`, `cmd/daemon.go`

### Phase 3: Medium-priority improvements (next sprint)

10. **Remove dead `internal/config` + Viper**
    - Finding: M1
    - Fix: Delete `internal/config/`, remove `github.com/spf13/viper` from `go.mod`, `go mod tidy`.
    - Effort: 0.5 day
    - Files: `internal/config/`, `go.mod`

11. **Daemon self-cleanup on crash**
    - Finding: M9
    - Fix: In `runDaemon`, defer cleanup of PID and socket files before the signal-wait loop.
    - Effort: 0.5 day
    - Files: `cmd/daemon.go`

12. **Test `DiscoverTools`/`InvokeTool`**
    - Finding: M6
    - Fix: Add tests using `net.Pipe()` mock + `attachConnectorFn` injection.
    - Effort: 1 day
    - Files: `internal/mcp/dynamic_test.go` (new)

13. **Structured logging**
    - Finding: H6
    - Fix: Replace `log.Printf` with `slog` (stdlib since Go 1.21). Add levels + connector name as structured field.
    - Effort: 2 days
    - Files: `cmd/daemon.go`, `cmd/*.go`

14. **Use `crypto/rand` for request IDs**
    - Finding: M8
    - Fix: Replace `time.Now().UnixNano()` with `crypto/rand` hex string.
    - Effort: 0.5 day
    - Files: `internal/mcp/dynamic.go`, `cmd/call.go`

### Phase 4: Low-priority cleanup (when touching these files)

15. **Replace `contains`/`containsSub` test helpers with `strings.Contains`**
    - Finding: testing agent
    - Files: `internal/executor/daemon_test.go`

16. **Remove busy-wait in logs follow mode**
    - Finding: architecture agent
    - Files: `cmd/logs.go`

### Dependency Graph

- Fix 6 (extract daemon.go) depends on Fix 3 (error returns) — extract after error handling is clean
- Fix 12 (test dynamic.go) depends on Fix 6 — easier to test once logic is in internal/
- Fix 13 (structured logging) depends on Fix 6 — add logging to extracted packages

### Estimated Total Effort

- Phase 1: 2 days
- Phase 2: 6.5 days
- Phase 3: 4.5 days
- Phase 4: 0.5 days (opportunistic)
- **Total: ~13.5 days**
