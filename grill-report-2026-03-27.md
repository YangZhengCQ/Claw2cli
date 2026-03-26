---
plugin: grill
version: 1.2.0
date: 2026-03-27
target: /Users/headless/code/Claw2cli
style: Hard-Nosed Critique + Roadmap
addons: Scale stress, Hidden costs, Principle violations, Strangler fig, Success metrics, Before vs after, Assumptions audit, Compact & optimize
agents: architecture, error-handling, security, testing
---

# Claw2Cli Grill Report (Post-PR #1 Merge)

## Hard-Nosed Critique

### Critical Flaws (4)

**C1. Sandbox is theater — fail-open on all platforms** `[CRITICAL]`
- Linux: `sandbox_linux.go` is a complete no-op (0 LOC of actual enforcement)
- macOS: Uses deprecated `sandbox-exec`; if it fails, execution proceeds unsandboxed
- Daemon: `daemon.go:133-137` logs warning and continues on sandbox failure
- Permission checks: advisory-only (`permission.go:11-14`), never enforced at OS level
- **Impact:** Users believe they have sandboxing; they do not. Any plugin has full user privileges.

**C2. npm install runs arbitrary code before any protection** `[CRITICAL]`
- `store/store.go:61` — `npm install` executes pre/post-install hooks with full user privileges
- No `--ignore-scripts` flag
- Happens before checksum is recorded, before sandbox is applied
- **Impact:** Supply chain attack vector with zero mitigation.

**C3. `store` vs `nodeutil` contradictory install models** `[CRITICAL]`
- `c2c install` uses `store.Install()` (local `node_modules`)
- `c2c update` uses `nodeutil.EnsurePluginInstalled()` (global `npm install -g`)
- `c2c connect` uses `store.ResolveTsx()` but `nodeutil.ResolveGlobalNodeModules()` as fallback
- **Impact:** `c2c update wechat` updates the global package but not the local `node_modules` that `c2c connect` actually uses. Updated plugin never takes effect.

**C4. Unbounded memory on skill output** `[CRITICAL]`
- `runner.go:58-59` — size check happens AFTER `cmd.Run()` completes
- Output captured into unbounded `strings.Builder`
- Malicious plugin can OOM the host before the 10MB check triggers
- **Impact:** Denial of service.

### High-Severity Issues (8)

| # | Finding | File |
|---|---------|------|
| H1 | UDS socket no authentication (0600 perms only) | `daemon.go:284-374` |
| H2 | Sandbox profile path injection via `fs:/` permission | `sandbox_darwin.go:71-74` |
| H3 | `cmd/daemon.go` god file (415 LOC, 7 responsibilities) | `cmd/daemon.go` |
| H4 | Goroutine leaks in DiscoverTools/InvokeTool (no SetReadDeadline) | `mcp/dynamic.go:39-64,108-138` |
| H5 | Shim stdin silently drops all malformed NDJSON | `plugin-sdk/index.js:78-80` |
| H6 | sandbox 0% test coverage on security-critical code | `internal/sandbox/` |
| H7 | store Install/Verify 20.7% coverage, core flow untested | `internal/store/` |
| H8 | Sandbox temp profile files never cleaned up | `sandbox_darwin.go:83-94` |

---

## 80/20 Rewrite Plan

**Don't rewrite.** The architecture is sound (9 clean packages, correct dependency direction, good protocol design). Focus on:

1. **Unify store/nodeutil** — Delete `nodeutil.EnsurePluginInstalled`, make `c2c update` use `store.Install`
2. **Add `--ignore-scripts` to npm install** — One line change, eliminates C2
3. **Fix runner output limiting** — Use `io.LimitedReader` wrapper
4. **Make sandbox non-optional** — If sandbox fails, refuse to run (or require `--no-sandbox` acknowledgment)

---

## Prioritized 15-Item Backlog

| Rank | Item | Severity | Impact | Effort |
|------|------|----------|--------|--------|
| 1 | Add `--ignore-scripts` to `store.Install()` npm command | CRITICAL | Supply chain | 0.5d |
| 2 | Unify store/nodeutil: make `c2c update` use store.Install | CRITICAL | Data consistency | 1d |
| 3 | Use `io.LimitedReader` for skill output capture | CRITICAL | DoS prevention | 0.5d |
| 4 | Validate `fs:` permission paths (no `/`, canonicalize) | HIGH | Sandbox escape | 0.5d |
| 5 | Set `conn.SetReadDeadline` in DiscoverTools/InvokeTool | HIGH | Goroutine leak | 0.5d |
| 6 | Add sandbox tests (profile generation, path injection) | HIGH | Test gap | 1d |
| 7 | Add store Install/Verify tests with mocked npm | HIGH | Test gap | 1d |
| 8 | Clean up sandbox temp profile after daemon start | HIGH | Resource leak | 0.5d |
| 9 | Remove global npm fallback from NODE_PATH | MEDIUM | Isolation | 0.5d |
| 10 | Block `NPM_CONFIG_REGISTRY` and other npm config vars | MEDIUM | Registry hijack | 0.5d |
| 11 | Extract daemon.go into internal/daemon package | MEDIUM | Testability | 2d |
| 12 | Deduplicate UDS client pattern (4 locations) | MEDIUM | Maintainability | 1d |
| 13 | Make golangci-lint blocking in CI | MEDIUM | Quality gate | 0.5d |
| 14 | Add structured logging (slog) | MEDIUM | Observability | 2d |
| 15 | Add macOS CI runner for sandbox compilation | LOW | CI coverage | 0.5d |

---

## Red Flags

1. **`c2c update` is functionally broken** — updates global packages that local store ignores
2. **59 golangci-lint errors are non-blocking** — linting is decorative, not enforced
3. **Sandbox advertised in docs but doesn't work on Linux** — misleading security posture

## Quick Wins

### < 1 day
- Add `--ignore-scripts` to npm install (1 line)
- Use `io.LimitedReader` for skill output
- Clean up sandbox temp files (add `defer os.Remove`)
- Set `conn.SetReadDeadline` in dynamic.go (2 locations)
- Block dangerous NPM_CONFIG_* env vars

### < 1 week
- Unify store/nodeutil, fix `c2c update`
- Add sandbox + store tests
- Validate `fs:` permission paths

---

## Add-on: Scale Stress
If traffic grows 100x: The UDS broadcast in daemon.go uses `sync.Map.Range` with `conn.Write` inline — a slow client blocks all broadcasts. Need per-client write queues or drop-on-slow.

## Add-on: Hidden Costs
1. **Debugging**: No structured logs, no correlation IDs, 3 different log approaches
2. **Onboarding**: daemon.go 415 LOC with 4 goroutines — new devs can't reason about it
3. **Store/nodeutil confusion**: Two npm install paths, unclear which to use
4. **Sandbox false security**: Users think they're protected; they're not on Linux
5. **CI noise**: 59 lint errors in every run, everyone learns to ignore CI

## Add-on: Principle Violations
- **SRP**: daemon.go has 7 responsibilities
- **Dependency Inversion**: cmd/ directly imports 8/9 internal packages
- **Least Privilege**: sandbox fail-open violates principle by design
- **DRY**: store/nodeutil duplicate version stripping, checksum fetching, CLI suffix logic

## Add-on: Assumptions Audit
1. **"npm is always available"** — checked at install, not at connect/run
2. **"tsx can handle all plugins"** — fails if plugin needs native modules
3. **"One account per plugin"** — handleInvokeTool picks first account
4. **"Sandbox-exec works on macOS"** — deprecated, may be removed in future macOS
5. **"Global npm packages are safe to include in NODE_PATH"** — undermines isolation

## Add-on: Compact & Optimize
- Delete `nodeutil.EnsurePluginInstalled` (replaced by store)
- Delete `nodeutil.ResolveNodeRunner` (replaced by store.ResolveTsx)
- Merge `store.stripVersion` + `store.stripCLISuffix` into `nodeutil.ResolvePluginPackage`
- Remove `channel-config-schema.js` files (wildcard exports make them unnecessary)
- Remove `internal/config` references from docs-guardian config.json

## Add-on: Success Metrics
- **MTTR**: Currently unmeasurable (no structured logs). Target: < 5 min with correlation IDs
- **Test coverage**: Currently 62% overall. Target: 80% with sandbox > 50%
- **Lint errors**: Currently 59. Target: 0 (blocking in CI)
- **Plugin load success rate**: Track how many plugins load vs fail

## Add-on: Before vs After
```
BEFORE (current):                    AFTER (target):
cmd/daemon.go (415 LOC)              internal/daemon/server.go (UDS)
  ├─ subprocess mgmt                 internal/daemon/bridge.go (shim I/O)
  ├─ UDS server                      internal/daemon/shutdown.go
  ├─ NDJSON routing                  cmd/daemon.go (20 LOC, just wires)
  ├─ client mgmt
  ├─ shutdown                        store + nodeutil merged:
  └─ 4 goroutines                    internal/store/ (single npm interface)

npm install → global + local         npm install --ignore-scripts → local only
sandbox: fail-open                   sandbox: fail-closed (--no-sandbox to override)
```

## Add-on: Strangler Fig
No big-bang needed. Incremental fixes:
1. Week 1: Add `--ignore-scripts`, fix output limiting, unify store
2. Week 2: Add tests for sandbox + store
3. Week 3: Extract daemon.go, add structured logging
4. Week 4: Make sandbox fail-closed, lint blocking

---

## Executive Summary

### One-Paragraph Verdict
Claw2Cli has a well-designed internal architecture (9 clean packages, correct dependency flow, good protocol) wrapped in an execution layer that doesn't enforce the security model it advertises. The sandbox is fail-open, npm install runs arbitrary code before any protection, and the `store` vs `nodeutil` split means `c2c update` is functionally broken. The post-PR-merge state is better than pre-merge (local store, sandbox framework, env filtering) but the implementation has significant gaps.

### Top 3 Actions
1. **Add `--ignore-scripts` to npm install** — eliminates the #1 supply chain vector in 1 line
2. **Unify store/nodeutil and fix `c2c update`** — the update command is broken, confusing, and a trust violation
3. **Add sandbox + store tests** — 0% and 20.7% coverage on security-critical code is unacceptable

### Confidence Level
| Recommendation | Confidence | What increases it |
|----------------|-----------|-------------------|
| --ignore-scripts | High | N/A, clearly needed |
| Unify store/nodeutil | High | Verify `c2c update` actually broken E2E |
| Sandbox tests | High | N/A |
| Fail-closed sandbox | Medium | Need to test all plugins with strict sandbox |
| Extract daemon.go | Medium | Profile developer confusion first |

---

## Fixing Plan

### Phase 1: Critical fixes (do immediately)
1. **C2: npm install runs arbitrary code** — Add `"--ignore-scripts"` to `store/store.go:61` npm command. Effort: 0.5d. Files: `internal/store/store.go`
2. **C3: store/nodeutil split breaks update** — Make `cmd/update.go` use `store.Install()` instead of `nodeutil.EnsurePluginInstalled()`. Effort: 1d. Files: `cmd/update.go`, `internal/store/store.go`
3. **C4: Unbounded skill output** — Wrap `cmd.Stdout`/`cmd.Stderr` with `io.LimitedReader` in `runner.go`. Effort: 0.5d. Files: `internal/executor/runner.go`

### Phase 2: High-priority fixes (this sprint)
4. **H2: Sandbox path injection** — Validate `fs:` paths: reject `/`, `..`, canonicalize. Effort: 0.5d. Files: `internal/sandbox/sandbox_darwin.go`, `internal/executor/permission.go`
5. **H4: Goroutine leaks** — Add `conn.SetReadDeadline` in `DiscoverTools` and `InvokeTool`. Effort: 0.5d. Files: `internal/mcp/dynamic.go`
6. **H6: Sandbox tests** — Test profile generation, path injection, network deny. Effort: 1d. Files: `internal/sandbox/sandbox_darwin_test.go` (new)
7. **H7: Store tests** — Test Install/Verify with mocked npm. Effort: 1d. Files: `internal/store/store_test.go`
8. **H8: Sandbox temp cleanup** — `defer os.Remove(profilePath)` after sandbox.Apply. Effort: 0.5d. Files: `internal/sandbox/sandbox_darwin.go`

### Phase 3: Medium-priority improvements (next sprint)
9. **Remove global npm fallback** from daemon.go NODE_PATH. Files: `cmd/daemon.go`
10. **Block NPM_CONFIG_*** env vars. Files: `internal/executor/environment.go`
11. **Extract daemon.go** into `internal/daemon/`. Files: `cmd/daemon.go` → new package
12. **Deduplicate UDS client pattern**. Files: `cmd/call.go`, `cmd/info.go`, `mcp/dynamic.go`, `mcp/server.go`
13. **Make lint blocking in CI**. Files: `.github/workflows/ci.yml`
14. **Add structured logging**. Files: multiple

### Phase 4: Low-priority cleanup
15. **Delete redundant nodeutil functions** (EnsurePluginInstalled, ResolveNodeRunner)
16. **Remove channel-config-schema.js** files (wildcard handles it)
17. **Add macOS CI runner**
18. **Fix 59 lint errors incrementally**

### Estimated Total Effort
- Phase 1: 2 days
- Phase 2: 3.5 days
- Phase 3: 5 days
- Phase 4: 2 days (opportunistic)
- **Total: ~12.5 days**
