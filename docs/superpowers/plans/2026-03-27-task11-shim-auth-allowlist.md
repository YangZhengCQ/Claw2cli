# Task 11: Shim Auth Allowlist — TDD Completion Plan

> **Parent plan:** `docs/superpowers/plans/2026-03-23-architectural-hardening-plan.md` → Phase 4, Task 11

## Context

The architectural hardening plan (Phase 4, Task 11) requires replacing always-allow auth with a configurable sender allowlist in the shim SDK. **The implementation already exists** in `shim/node_modules/@openclaw/plugin-sdk/index.js` (lines 461-534) but has **zero tests**. The TDD Guardian workflow requires comprehensive behavior tests before marking this complete.

### Key Discovery: Empty-Array Semantic Divergence

- `checkSenderAllowlist([])` → **blocks all** (empty array is truthy, `[].includes(x)` is always false)
- `isNormalizedSenderAllowed(id, [])` → **allows all** (early return when `allowList.length === 0`)

Tests must document this explicitly.

---

## Files

| File | Action |
|------|--------|
| `shim/test/auth.test.js` | **Create** — all auth allowlist tests |
| `shim/node_modules/@openclaw/plugin-sdk/index.js` | Read-only (implementation already done, lines 461-534) |

---

## Step 1: Create `shim/test/auth.test.js`

Using `node:test` + `node:assert` (matching existing test style in `sdk.test.js`).

### Suite A: `resolveSenderCommandAuthorization(ctx)` → `{ authorized: bool }`

Tested via exported function; exercises internal `checkSenderAllowlist()`.

| # | Scenario | globalConfig | ctx | Expected |
|---|----------|-------------|-----|----------|
| 1 | no config key → allow all | `{}` | `{ senderId: "u1" }` | `{ authorized: true }` |
| 2 | null → allow all | `{ authorized_senders: null }` | `{ senderId: "u1" }` | `{ authorized: true }` |
| 3 | non-array (string) → allow all | `{ authorized_senders: "u1" }` | `{ senderId: "u1" }` | `{ authorized: true }` |
| 4 | empty array → block all | `{ authorized_senders: [] }` | `{ senderId: "u1" }` | `{ authorized: false }` |
| 5 | listed sender → allow | `{ authorized_senders: ["u1","u2"] }` | `{ senderId: "u1" }` | `{ authorized: true }` |
| 6 | unlisted sender → block | `{ authorized_senders: ["u1"] }` | `{ senderId: "u2" }` | `{ authorized: false }` |

### Suite B: senderId extraction priority

| # | Scenario | ctx | allowlist | Expected |
|---|----------|-----|-----------|----------|
| 7 | prefers senderId | `{ senderId:"a", from:"b", From:"c" }` | `["a"]` | authorized |
| 8 | falls back to from | `{ from:"b", From:"c" }` | `["b"]` | authorized |
| 9 | falls back to From | `{ From:"c" }` | `["c"]` | authorized |
| 10 | senderId takes precedence (negative) | `{ senderId:"a", from:"b" }` | `["b"]` | blocked |

### Suite C: `resolveSenderCommandAuthorizationWithRuntime(runtime, ctx)`

| # | Scenario | Notes |
|---|----------|-------|
| 11 | allows listed sender | runtime param is ignored by impl |
| 12 | blocks unlisted sender | same |
| 13 | runtime value irrelevant | pass different runtimes, same result |

### Suite D: `resolveDirectDmAuthorizationOutcome(ctx, config)` → `{ allowed: bool }`

| # | Scenario | Expected |
|---|----------|----------|
| 14 | no authorized_senders → allow | `{ allowed: true }` |
| 15 | empty array → block | `{ allowed: false }` |
| 16 | listed → allow | `{ allowed: true }` |
| 17 | unlisted → block | `{ allowed: false }` |
| 18 | config param does NOT override globalConfig | pass config with allowlist, globalConfig empty → still allows |

### Suite E: `isNormalizedSenderAllowed(senderId, allowList)` (explicit param, no globalConfig)

| # | Scenario | Expected |
|---|----------|----------|
| 19 | null allowList → allow | `true` |
| 20 | undefined allowList → allow | `true` |
| 21 | **empty array → allow** (diverges from checkSenderAllowlist!) | `true` |
| 22 | listed → allow | `true` |
| 23 | unlisted → block | `false` |

### Suite F: Return shape contracts

| # | Scenario | Assertion |
|---|----------|-----------|
| 24 | command auth returns `{ authorized }` only | `Object.keys(result)` → `["authorized"]` |
| 25 | DM auth returns `{ allowed }` only | `Object.keys(result)` → `["allowed"]` |

---

## Step 2: Run tests

```bash
node --test shim/test/auth.test.js    # new auth tests
node --test shim/test/                 # full shim test suite (no regressions)
```

All 25 tests should pass against existing implementation.

## Step 3: Handle stdout noise

The auth functions call `sendLog()` which writes to `process.stdout`. If this interferes with test output, suppress by setting `stdoutClosed = true` in beforeEach (not exported — or just tolerate the noise since `node:test` handles it).

---

## Verification

- [x] `node --test shim/test/auth.test.js` — 25 tests pass
- [x] `node --test shim/test/*.test.js` — all shim tests pass (no regressions)
- [x] `go test -race ./...` — Go tests unaffected

> Completed 2026-03-27. All 25 tests pass against existing implementation.
