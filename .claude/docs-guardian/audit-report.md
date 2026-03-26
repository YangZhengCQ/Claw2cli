# Documentation Audit Report

**Project**: Claw2Cli
**Date**: 2026-03-27
**Language**: Go + Node.js
**Framework**: Cobra CLI + MCP

## Executive Summary

| Dimension | Score | Status |
|-----------|-------|--------|
| Freshness | 85/100 | 🟡 |
| Accuracy  | 68/100 | 🔴 |
| Coverage  | 85%    | 🟡 |
| Quality   | 92/100 | 🟢 |

**Overall health**: 78/100

Post-PR #1 merge significantly changed the architecture (local store replaces npx, sandbox added, config deleted) but documentation was not updated. The code is sound but the docs describe a different system.

## Critical Findings (fix immediately)

### 1. [CRITICAL] Skill execution model: docs say npx, code uses local store
- **Docs**: DESIGN.md §4.2, IMPLEMENTATION.md §8 describe npx-based skill execution
- **Code**: `runner.go` uses `store.New()` + `store.ResolveTsx()` + local `node_modules/.bin/`
- **Files**: `docs/DESIGN.md:99-111`, `docs/IMPLEMENTATION.md:260`

### 2. [CRITICAL] Project structure lists deleted package, missing 2 new packages
- **Docs**: IMPLEMENTATION.md §1 lists `internal/config/config.go` (deleted)
- **Missing**: `internal/store/` (local npm management) and `internal/sandbox/` (OS sandboxing)
- **File**: `docs/IMPLEMENTATION.md:38-63`

### 3. [CRITICAL] Test coverage table completely stale
- **Docs**: Claims executor 91.4%, mcp 43.1%, paths 95.7%
- **Actual**: executor 82.0%, mcp 69.2%, paths 62.9%
- **Missing**: `internal/store` (20.7%), `internal/sandbox` (0%)
- **File**: `docs/IMPLEMENTATION.md:303-315`

### 4. [HIGH] Connector daemon now uses store + sandbox, docs describe old flow
- **Docs**: IMPLEMENTATION.md §4 describes `nodeutil.EnsurePluginInstalled()` + `nodeutil.ResolveNodeRunner()`
- **Code**: `daemon.go` uses `store.New()` + `store.ResolveTsx()` + `sandbox.Apply()`
- **File**: `docs/IMPLEMENTATION.md:112-130`

### 5. [HIGH] Key functions table references wrong packages
- **Docs**: Lists `ResolveNodeRunner()` at `internal/nodeutil`
- **Code**: Primary interface is now `store.ResolveTsx()` at `internal/store/tsx.go`
- **File**: `docs/IMPLEMENTATION.md:278-300`

### 6. [HIGH] Storage layout shows config.yaml that doesn't exist
- **Docs**: `~/.c2c/config.yaml — Global configuration` in DESIGN.md §4.7
- **Code**: config package deleted, Viper removed, no config.yaml anywhere
- **File**: `docs/DESIGN.md:237`

## Medium Findings (fix soon)

### 7. [MEDIUM] SDK wildcard exports not documented
- package.json `"./*": "./index.js"` pattern undocumented in DESIGN.md §4.9

### 8. [MEDIUM] store.CleanupReplacedPackages not in any docs
- Removes openclaw/clawdbot/@mariozechner from local node_modules to prevent pi-ai conflicts

### 9. [MEDIUM] `--no-sandbox` flag not in README CLI reference

### 10. [MEDIUM] Roadmap Phase 3 lists "Remove internal/config + Viper" as future — already done

### 11. [MEDIUM] New protocol message types undocumented
- `TypePing`, `TypePong`, `TypeShutdown` — no doc comments, not in DESIGN.md §4.3

### 12. [MEDIUM] PluginManifest new fields `ResolvedVersion` and `Integrity` lack doc comments

## Low Findings (nice to have)

- Architecture diagrams use inconsistent ASCII styles
- README_CN.md has orphaned text from diagram restructure
- PITFALLS.md not cross-referenced in English README
- MessageType constants lack individual doc comments (9 values)
- PluginTypeSkill/PluginTypeConnector constants undocumented

## Fixing Plan

1. **Update DESIGN.md + IMPLEMENTATION.md**: Replace all npx references with local store model. Describe `store.New()`, `store.ResolveTsx()`, `store.Install()`, `store.CleanupReplacedPackages()`.
2. **Update IMPLEMENTATION.md §1**: Remove `internal/config`, add `internal/store/` and `internal/sandbox/`.
3. **Update test coverage table**: Run coverage, update all 10 package numbers.
4. **Update IMPLEMENTATION.md §4**: Daemon now uses store + sandbox. Document `--no-sandbox` flag.
5. **Update key functions table**: Add store functions, update file locations.
6. **Remove config.yaml from DESIGN.md §4.7 storage layout**.
7. **Mark Phase 3 config/Viper removal as done in roadmap**.
8. **Document SDK wildcard exports** in DESIGN.md §4.9.
9. **Add `--no-sandbox` to README CLI reference**.
10. **Add doc comments to new protocol types and manifest fields**.
