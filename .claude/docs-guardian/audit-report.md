# Documentation Audit Report

**Project**: Claw2Cli
**Date**: 2026-03-23
**Language**: Go
**Framework**: None (Cobra CLI)

## Executive Summary

| Dimension | Score | Status |
|-----------|-------|--------|
| Freshness | 98/100 | 🟢 |
| Accuracy  | 62/100 | 🔴 |
| Coverage  | 92%    | 🟡 |
| Quality   | 80/100 | 🟡 |

**Overall health**: 78/100

## Critical Findings (fix immediately)

### 1. [CRITICAL] `foregroundMode` variable name inverted in connect.go
- **Source**: `cmd/connect.go:11,33`
- **Issue**: Variable named `foregroundMode` is set by the `--background/-b` flag. When `-b` is passed, `foregroundMode=true` triggers background mode. The variable name contradicts its actual behavior.
- **Fix**: Rename to `backgroundMode` to match the flag semantics.

### 2. [CRITICAL] `c2c echo` command missing from README CLI Reference
- **Source**: `cmd/echo.go` (fully implemented)
- **Doc**: `README.md` CLI Reference table (lines 45-56)
- **Issue**: The `echo` command is registered and publicly callable but completely absent from documentation. Users cannot discover it.
- **Fix**: Add row to CLI Reference: `c2c echo <connector>` — "Test consumer that echoes back received messages"

### 3. [HIGH] Install pre-flight checks not mentioned in README
- **Source**: `cmd/install.go:38-45` (`checkNodeNpm`, `checkShimFiles`)
- **Doc**: `README.md` Quick Start section
- **Issue**: DESIGN.md mentions pre-checks conceptually, but README Quick Start gives no indication that `c2c install` validates the environment first.
- **Fix**: Add a note in Quick Start explaining pre-flight validation.

### 4. [HIGH] `resolveNodeRunner()` auto-install behavior undocumented
- **Source**: `cmd/daemon.go:306-325`
- **Doc**: `docs/DESIGN.md` §4.4, §4.9
- **Issue**: The function auto-installs tsx globally if not found. This side-effect is not documented. Users may be surprised by a global npm install.
- **Fix**: Document the auto-install behavior in DESIGN.md §4.9.

### 5. [HIGH] AttachConnector foreground fallback not documented
- **Source**: `internal/executor/daemon.go:199-220`
- **Doc**: `docs/DESIGN.md` §4.4
- **Issue**: The PID-less socket fallback for foreground mode is not explained in docs.
- **Fix**: Add note in §4.4 that `attach` works for both foreground and background modes.

### 6. [HIGH] EPIPE handling in shim not documented
- **Source**: `shim/node_modules/@openclaw/plugin-sdk/index.js:22-27`
- **Doc**: `docs/DESIGN.md` §4.9
- **Issue**: Graceful stdout closure handling is a reliability feature but not mentioned in docs.
- **Fix**: Add under §4.9 Runtime 函数分类 or communication section.

## Medium Findings (fix soon)

### 7. [MEDIUM] `config.yaml` fields undocumented
- **Source**: `internal/config/config.go` — `C2CConfig`, `Load()`
- **Issue**: No documentation describes what config keys are available (`default_timeout` etc.).
- **Fix**: Add a "Configuration" section to README or DESIGN.md.

### 8. [MEDIUM] `ConnectorStatus` JSON shape undocumented for MCP consumers
- **Source**: `internal/executor/daemon.go:19`
- **Issue**: MCP `status` action returns this struct as JSON but field names/types not in docs.
- **Fix**: Document the JSON schema in DESIGN.md §4.8.

### 9. [MEDIUM] Placeholder GitHub URL in README
- **Source**: `README.md:17`
- **Issue**: `go install github.com/user/claw2cli@latest` — should be `github.com/YangZhengCQ/Claw2cli`.
- **Fix**: Update the URL.

### 10. [MEDIUM] require() fallback not mentioned in docs
- **Source**: `shim/c2c-shim.js:69-74`
- **Doc**: `docs/DESIGN.md` §4.2
- **Issue**: Docs mention ESM `import()` but not the CommonJS `require()` fallback.
- **Fix**: Add note about CJS compatibility fallback.

### 11. [MEDIUM] DESIGN.md lacks Table of Contents
- **Issue**: 449-line document with no TOC. Navigation is difficult.
- **Fix**: Add anchor-linked TOC after the intro.

## Low Findings (nice to have)

- README.md missing "last updated" date marker
- CLI Reference table could include output format notes
- DESIGN.md coverage metrics lack version context ("as of v0.1.0")
- DESIGN.md NDJSON examples could use inline sender/receiver annotations
- Mixed Chinese/English in DESIGN.md (valid choice, but a language note at top would help)

## Fixing Plan

Priority-ordered actions:

1. **Rename `foregroundMode` → `backgroundMode`** in `cmd/connect.go` (critical bug, 1 minute)
2. **Add `c2c echo` to README CLI Reference table** (critical gap, 1 minute)
3. **Fix placeholder URL** `github.com/user/claw2cli` → `github.com/YangZhengCQ/Claw2cli` in README (1 minute)
4. **Add pre-flight checks note** to README Quick Start section
5. **Document tsx auto-install** and EPIPE handling in DESIGN.md §4.9
6. **Document AttachConnector fallback** in DESIGN.md §4.4
7. **Add require() fallback note** in DESIGN.md §4.2
8. **Add Configuration section** documenting `config.yaml` fields
9. **Add ConnectorStatus JSON schema** in DESIGN.md §4.8
10. **Add Table of Contents** to DESIGN.md

## Full Agent Reports

<details>
<summary>Staleness Report</summary>

**Result**: All 18 mapped pairs are current (maximum staleness: 12 minutes). No stale documentation found. Documentation commits closely follow code commits.

**Score**: 98/100 (2 points deducted for cmd/echo.go having no doc mapping at all)

</details>

<details>
<summary>Accuracy Report</summary>

**Critical mismatches**: 2
- connect.go: `foregroundMode` flag logic inverted (variable name contradicts behavior)
- install.go: pre-flight checks not documented in README

**High severity**: 4
- daemon.go: `resolveNodeRunner()` auto-install not documented
- daemon.go: `resolvePluginPackage()` under-documented
- executor/daemon.go: AttachConnector fallback logic not documented
- plugin-sdk/index.js: EPIPE handling not documented

**Medium severity**: 3
- echo.go: command not in CLI Reference
- c2c-shim.js: require() fallback not mentioned
- install.go: preInstallPackage strategy not detailed

**Overall accuracy rate**: 62.5% (5 of 8 major mappings fully accurate)

</details>

<details>
<summary>Coverage Report</summary>

**Total exported symbols**: 53
**Inline doc coverage**: 100% (all symbols have godoc comments)
**External doc coverage (CLI commands)**: 91.7% (11/12 commands documented; `echo` missing)
**Config field documentation**: 0% (`default_timeout` not mentioned)

**Key gaps**:
- `c2c echo` command absent from README
- `C2CConfig` and `Load()` not in external docs
- `ConnectorStatus` JSON shape undocumented for MCP consumers

</details>

<details>
<summary>Quality Report</summary>

**README.md**: 82/100
- Good structure, clear Quick Start, helpful architecture diagram
- Gaps: placeholder URL, missing date marker

**DESIGN.md**: 78/100
- Comprehensive 449-line technical spec with excellent depth
- Gaps: no TOC, mixed language without declaration, vague phase timelines

**Average quality**: 80/100
**Total issues**: 7 (0 critical, 0 high, 2 medium, 5 low)

</details>
