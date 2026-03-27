# Documentation Audit Report

**Project**: Claw2Cli
**Date**: 2026-03-28
**Language**: Go 1.23 + Node.js (shim)
**Framework**: Cobra CLI + MCP

## Executive Summary

| Dimension | Score | Status |
|-----------|-------|--------|
| Freshness | 100/100 | :green_circle: |
| Accuracy  | 85/100 | :yellow_circle: |
| Coverage  | 98.6% | :green_circle: |
| Quality   | 85/100 | :yellow_circle: |

**Overall health**: 92/100

## Critical Findings (fix immediately)

### 1. [CRITICAL] CLI Reference missing `c2c update` command
**File**: `README.md` CLI Reference table
**Issue**: `cmd/update.go` defines a full `c2c update` command registered in `root.go:38`, but the README CLI Reference table does not list it. Users cannot discover this feature from documentation.
**Fix**: Add row to CLI Reference table: `c2c update [plugin]` | Update installed plugins to latest version

### 2. [HIGH] DESIGN.md Section 3.2 contradicts actual `connect` default
**File**: `docs/DESIGN.md` line 59
**Issue**: Says "foreground by default, `-b` for background" but code uses background by default with `-f` for foreground. Other sections of the same file (4.4, 7) are correct.
**Fix**: Change line 59 to: `c2c connect <connector>` (background daemon by default, `-f` for foreground)

### 3. [HIGH] DESIGN.md inconsistent shutdown timeouts across sections
**File**: `docs/DESIGN.md` lines 176, 183, 555
**Issue**: Three different timeout values (9s, 5s, 3s) documented in different sections. Code uses 9s for shim subprocess and 5s for PID-based StopConnector.
**Fix**: Line 555 should read "SIGTERM -> 9s for shim, 5s for PID-based fallback -> SIGKILL"

## Medium Findings (fix soon)

| # | Finding | File | Fix |
|---|---------|------|-----|
| 4 | Storage layout doesn't mention `logs/` and `bin/` are on-demand | `docs/DESIGN.md` 4.7 | Add note: these dirs are created on-demand, not by EnsureDirs |
| 5 | Install flow missing `--skip-verify` flag docs | `docs/IMPLEMENTATION.md` S3 | Add skip-verify and retry-on-connect behavior |
| 6 | `--ignore-scripts` security behavior undocumented | README + IMPLEMENTATION.md | Document why npm install uses --ignore-scripts |
| 7 | Linux sandbox placeholder text in DESIGN.md and IMPLEMENTATION.md | Both files | Move to "Future Work" section or mark clearly |
| 8 | README.md and README_CN.md missing last-updated dates | Both files | Add `> Last updated: 2026-03-28` |

## Low Findings (nice to have)

- README.md and README_CN.md lack formal table of contents (>100 lines each)
- README_CN.md architecture diagram has extra line vs English version
- PITFALLS.md is Chinese-only — English summary would help non-Chinese readers
- DESIGN.md Section 4.10 uses 4-level heading depth
- IMPLEMENTATION.md has one code block without language tag (line 76)
- `cmd/root.go:Execute()` is the only undocumented export (LOW — CLI internal)
- `internal/config/config.go` mapping in docs-guardian config is stale (file was removed)

## Fixing Plan

Priority-ordered:
1. **Add `c2c update` to README CLI Reference** — 2 min, CRITICAL
2. **Fix DESIGN.md Section 3.2 connect default** — 1 min, HIGH
3. **Fix DESIGN.md shutdown timeout table** — 5 min, HIGH
4. **Add last-updated dates to README.md and README_CN.md** — 2 min, MEDIUM
5. **Document `--ignore-scripts` and `--skip-verify` in install flow** — 10 min, MEDIUM
6. **Clarify on-demand dirs (logs/, bin/) in storage layout** — 5 min, MEDIUM
7. **Move Linux sandbox placeholder to Future Work** — 5 min, MEDIUM
8. **Remove stale config/config.go mapping from docs-guardian config** — 1 min, LOW

## Full Agent Reports

<details>
<summary>Staleness Report</summary>

**Status: No stale documentation detected (0/15 pairs stale)**

All 15 documentation mappings are current. Documentation files are consistently updated at or after the corresponding source code changes. Average staleness: -2.4 days (docs ahead of code).

| # | Source | Doc | Drift | Status |
|---|--------|-----|-------|--------|
| 1 | cmd/*.go | README.md | -4d | OK |
| 2 | internal/parser/types.go | DESIGN.md | -4d | OK |
| 3 | internal/parser/skillmd.go | DESIGN.md | -5d | OK |
| 4 | internal/parser/manifest.go | DESIGN.md | -4d | OK |
| 5 | internal/executor/runner.go | DESIGN.md | -1d | OK |
| 6 | internal/executor/daemon.go | DESIGN.md | -4d | OK |
| 7 | internal/executor/permission.go | DESIGN.md | -4d | OK |
| 8 | internal/mcp/server.go | DESIGN.md | -1d | OK |
| 9 | internal/mcp/converter.go | DESIGN.md | -4d | OK |
| 10 | internal/protocol/messages.go | DESIGN.md | -4d | OK |
| 11 | internal/protocol/codec.go | DESIGN.md | -4d | OK |
| 12 | internal/paths/paths.go | DESIGN.md | -4d | OK |
| 13 | cmd/install.go | README.md | -4d | OK |
| 14 | IMPLEMENTATION.md | standalone | 0d | OK |
| 15 | PITFALLS.md | standalone | 0d | OK |

</details>

<details>
<summary>Accuracy Report</summary>

**7 findings, accuracy rate ~85%**

1. **CRITICAL**: CLI Reference missing `c2c update` command — `cmd/update.go` exists but not in README table
2. **HIGH**: DESIGN.md Section 3.2 says foreground default, code uses background default with `-f`
3. **HIGH**: Three different shutdown timeout values (9s, 5s, 3s) across DESIGN.md sections
4. **MEDIUM**: Storage layout doesn't mention logs/ and bin/ are on-demand (not in EnsureDirs)
5. **MEDIUM**: Install flow missing `--skip-verify` flag and retry-on-connect behavior
6. **LOW**: Daemon shutdown timeout description ambiguous about which timeout applies where
7. **MEDIUM**: `--ignore-scripts` security behavior not documented

</details>

<details>
<summary>Coverage Report</summary>

**98.6% coverage (70/71 exported symbols documented)**

Only undocumented export: `cmd/root.go:Execute()` — LOW severity, CLI internal entry point.

All internal packages at 100% coverage:
- executor (6/6), mcp (6/6), nodeutil (6/6), parser (8/8), paths (11/11)
- protocol (17/17), registry (4/4), sandbox (3/3), store (5/5)

All 70 documented exports have proper Go doc comments and are referenced in IMPLEMENTATION.md Section 9 key functions table or DESIGN.md architectural sections.

</details>

<details>
<summary>Quality Report</summary>

**Average quality: 85/100**

| File | Score | Key Issues |
|------|-------|-----------|
| PITFALLS.md | 90/100 | Chinese-only, no English summary |
| DESIGN.md | 88/100 | Linux sandbox placeholder, deep nesting in 4.10 |
| IMPLEMENTATION.md | 85/100 | Linux sandbox placeholder, one untagged code block |
| README.md | 82/100 | No last-updated date, no TOC |
| README_CN.md | 80/100 | No last-updated date, no TOC, diagram divergence |

**Strengths**: Consistent heading hierarchy, valid internal links, comprehensive code examples, proper code block language tags, clear architectural documentation.

**Areas for improvement**: Add last-updated dates to READMEs, add formal TOCs, translate PITFALLS.md summary to English, address Linux sandbox placeholders.

</details>
