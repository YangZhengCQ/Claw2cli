# Architectural Hardening Design

**Date**: 2026-03-23
**Status**: Approved
**Scope**: 14 remaining audit findings requiring architectural solutions
**Security stance**: Defense in depth — plugins are untrusted code

---

## Context

A full 9-dimension audit found 64 issues. 51 were fixed with targeted patches. The remaining 14 cluster into 4 architectural themes that require coordinated design, not point fixes:

| Theme | Issues | Core Problem |
|-------|--------|-------------|
| A. Package Trust | #14, #22, #37, #38, #39 | Runtime fetching from global npm with no version pinning or integrity verification |
| B. Daemon Lifecycle | #25, #27, #28, #29 | PID files as sole coordination — no readiness, no identity verification, no reliable shutdown |
| C. Security Model | #21, #62 | Permissions and auth declared but never enforced |
| D. Interface & Tests | #19, #33, #64 | Skill args lose structure; cmd/ and shim have zero tests |

## Design Decisions

1. **Local Package Store** over global npm — eliminates runtime supply chain surface
2. **Socket-based supervisor** over PID files — identity verified via socket, not filesystem
3. **Two-layer security** — OS sandbox (landlock/sandbox-exec) + shim auth (configurable allowlist)
4. **Backwards-compatible arg parsing** — JSON arrays preferred, space-split fallback
5. **Three-tier testing** — unit + integration + property-based/fuzz

---

## Theme A: Local Package Store

### Problem

- `EnsurePluginInstalled` runs `npm install -g` at every `c2c connect` (daemon.go:81)
- Skills execute via `npx -y` which fetches remote code at runtime (runner.go:37)
- `tsx` auto-installed globally with user prompt (nodeutil.go:49)
- Version handling only special-cases `@latest` (nodeutil.go:77)
- `npm list -g` checks presence, not version match (nodeutil.go:82)

### Solution

New package `internal/store` manages per-plugin local package directories.

#### Directory structure

```
~/.c2c/
  bin/
    tsx                           # shared tsx binary, pinned version
  plugins/<name>/
    manifest.yaml                 # existing + new fields: resolved_version, integrity
    node_modules/                 # NEW: local npm install target
    SKILL.md                      # existing
```

#### New package: internal/store

```go
package store

// Store manages the local package directory for a plugin.
type Store struct {
    pluginDir string
}

// Install resolves the exact version, installs locally, and records integrity.
func (s *Store) Install(source string) error

// Verify checks that installed packages match manifest integrity hashes.
func (s *Store) Verify(manifest *parser.PluginManifest) error

// NodeModulesPath returns the path to the local node_modules directory.
func (s *Store) NodeModulesPath() string

// IsInstalled checks whether node_modules exists and is non-empty.
func (s *Store) IsInstalled() bool

// EnsureTsx installs tsx to ~/.c2c/bin/ if not present. Pinned to known version.
func EnsureTsx() (string, error)
```

#### Install flow changes (cmd/install.go)

1. `npm view <source> version --json` — resolve exact version
2. `npm install --prefix ~/.c2c/plugins/<name> <source>@<exact-version>` — local install
3. Verify integrity: compare `npm audit signatures` or computed checksum
4. Store `resolved_version` and `integrity` in manifest.yaml
5. Install tsx to `~/.c2c/bin/tsx` (shared, pinned to `tsx@4.19.4`)

#### Runtime flow changes (cmd/daemon.go — connectors)

1. **Remove** `nodeutil.EnsurePluginInstalled()` call — zero network at connect-time
2. Set `NODE_PATH` to `shimNodeModules:~/.c2c/plugins/<name>/node_modules` — two paths: the fake SDK directory (shim/node_modules) for `@openclaw/plugin-sdk` resolution, plus the local store for actual plugin packages
3. Use `~/.c2c/bin/tsx` as runner (not global tsx)
4. If local packages missing, fail: `"packages not found — run 'c2c install <name>' first"`

#### Runtime flow changes (internal/executor/runner.go — skills)

1. **Remove** `npx -y` execution model — skills no longer fetch remote code at runtime
2. `RunSkill` resolves the plugin's local node_modules path via `store.NodeModulesPath()`
3. Execute via `~/.c2c/bin/tsx <plugin-entry-point>` with `NODE_PATH` set to local store
4. If local packages missing, fail: `"skill not installed — run 'c2c install <name>' first"`
5. **Remove** `preInstallPackage()` from `cmd/install.go` — replaced by `store.Install()`

#### Manifest schema additions

```yaml
source: "@tencent-weixin/openclaw-weixin-cli@latest"
type: connector
resolved_version: "1.2.3"          # NEW: exact resolved version
integrity: "sha512:abc123..."       # NEW: verified integrity hash
permissions:
  - network
  - "fs:~/.c2c/storage/wechat"
checksum: "sha512:abc123..."        # existing (deprecated, replaced by integrity)
```

#### Migration

**Connectors**:
- Existing global installs continue working via NODE_PATH fallback (global node_modules appended as third path)
- `c2c install` creates local node_modules on next run
- `c2c connect` warns if using global fallback, suggests `c2c install --upgrade <name>`

**Skills**:
- Skills that were only installed globally must be reinstalled: `c2c install <skill-package>`
- `c2c run <skill>` checks for local node_modules first; if missing, fails with clear error

**Checksum → Integrity migration**:
- `parser.LoadPlugin` accepts both `checksum` and `integrity` fields during migration
- If both present, `integrity` takes precedence
- `store.Verify()` handles both formats
- New installs write `integrity` only; `checksum` is not written for new installs

---

## Theme B: Socket-based Daemon Lifecycle

### Problem

- `StartConnector` returns before daemon is ready (daemon.go:71)
- `StopConnector` trusts PID file blindly — recycled PIDs can target wrong process (daemon.go:119)
- `process.Wait()` doesn't work for detached daemons (daemon.go:130)
- Tests don't exercise detached-daemon behavior (daemon_test.go:342)

### Solution

UDS handshake protocol replaces PID-file authority. PID files become advisory metadata.

#### New protocol messages

```go
const (
    TypePing     MessageType = "ping"
    TypePong     MessageType = "pong"
    TypeShutdown MessageType = "shutdown"
)
```

#### StartConnector changes

After spawning the child:

1. Write PID file (advisory, for `c2c status` display)
2. Poll socket with exponential backoff: 50ms, 100ms, 200ms, 400ms, 1s, 2s
3. On socket connect: send `ping`, wait for `pong`
4. UDS socket permissions (0600) already ensure only the owning user can connect — no additional nonce needed for identity verification
5. If no pong within 10s: kill child, cleanup, return error
6. Return success only after confirmed readiness

```go
func waitForReady(name string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    backoff := 50 * time.Millisecond
    for time.Now().Before(deadline) {
        conn, err := net.Dial("unix", paths.SocketPath(name))
        if err != nil {
            time.Sleep(backoff)
            backoff = min(backoff*2, 2*time.Second)
            continue
        }
        defer conn.Close()
        msg := protocol.NewCommand("c2c", "ping", "ready-check", nil)
        data, _ := json.Marshal(msg)
        conn.Write(append(data, '\n'))
        conn.SetReadDeadline(time.Now().Add(3 * time.Second))
        scanner := bufio.NewScanner(conn)
        if scanner.Scan() {
            var resp protocol.Message
            if json.Unmarshal(scanner.Bytes(), &resp) == nil && resp.Type == protocol.TypePong {
                return nil
            }
        }
        time.Sleep(backoff)
        backoff = min(backoff*2, 2*time.Second)
    }
    return fmt.Errorf("daemon did not become ready within %s", timeout)
}
```

#### runDaemon changes

Handle new commands in the UDS client handler **before** the shim-forwarding branch (these are daemon-level commands, not plugin commands):
- `ping` → respond with `pong` immediately
- `shutdown` → trigger graceful shutdown (same path as SIGTERM)

These must be handled before the `list_tools` and shim-forwarding logic to prevent them from being sent to the plugin process.

#### StopConnector changes

Socket-first, PID-fallback:

1. Connect to UDS socket
2. Send `shutdown` command
3. Wait for socket close (daemon exited cleanly)
4. If socket unreachable: fall back to PID-based SIGTERM (stale daemon)
5. Cleanup files after confirmed exit

```go
func StopConnector(name string) error {
    conn, err := net.DialTimeout("unix", paths.SocketPath(name), 2*time.Second)
    if err == nil {
        defer conn.Close()
        msg := protocol.NewCommand("c2c", "shutdown", "stop-req", nil)
        data, _ := json.Marshal(msg)
        conn.Write(append(data, '\n'))
        conn.SetReadDeadline(time.Now().Add(5 * time.Second))
        buf := make([]byte, 1)
        conn.Read(buf) // blocks until close or deadline
        cleanupConnectorFiles(name)
        return nil
    }
    // Fallback: PID-based kill
    // ... existing logic as last resort ...
}
```

#### Test changes

- `TestStartConnector_WaitsForReady` — mock UDS listener responds to ping
- `TestStopConnector_ViaSocket` — real UDS listener, verify shutdown protocol
- `TestStopConnector_FallbackToPID` — no socket, verify PID-based fallback
- `TestStartConnector_TimeoutOnUnresponsiveDaemon` — socket exists but no pong

---

## Theme C: Security Model

### Problem

- `CheckPermissions` only validates syntax — plugins run with full OS privileges (#21)
- Auth helpers always return `{authorized: true}` — sender verification disabled (#62)

### Solution

Two-layer defense: OS sandbox for subprocess isolation + shim auth for message filtering.

### Layer 1: OS Sandbox (internal/sandbox/)

New package wrapping `exec.Command` with platform-specific restrictions.

#### Linux (landlock + seccomp)

```go
// internal/sandbox/sandbox_linux.go
func Apply(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
    var fsRules []landlock.PathAccess
    hasNetwork := false

    for _, p := range manifest.Permissions {
        switch {
        case string(p) == "network":
            hasNetwork = true
        case strings.HasPrefix(string(p), "fs:"):
            path := strings.TrimPrefix(string(p), "fs:")
            fsRules = append(fsRules, landlock.RWDirs(path))
        }
    }

    // Always allow: shim (RO), node_modules (RO), node binary (RO), tmp (RW)
    fsRules = append(fsRules,
        landlock.RODirs(paths.ShimDir),
        landlock.RODirs(paths.NodeModules),
        landlock.ROFiles(paths.NodeRunner),
        landlock.RWDirs(os.TempDir()),
    )

    if !hasNetwork {
        // Block network via seccomp: deny AF_INET/AF_INET6 connect
        applyNetworkBlock(cmd)
    }

    return landlock.V5.BestEffort().Restrict(fsRules...)
}
```

#### macOS (sandbox-exec)

**Note**: Apple has deprecated `sandbox-exec` and may remove it in future macOS versions. This implementation is best-effort and includes a runtime capability check. If `sandbox-exec` is unavailable, the fallback path (fail-open with warning) applies.

```go
// internal/sandbox/sandbox_darwin.go
func Apply(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
    // Runtime capability check: verify sandbox-exec works
    if err := exec.Command("/usr/bin/sandbox-exec", "-n", "no-internet", "/usr/bin/true").Run(); err != nil {
        return fmt.Errorf("sandbox-exec not available: %w", err)
    }
    profile := generateProfile(manifest.Permissions, paths)
    profilePath := writeTempProfile(profile)
    // Wrap: sandbox-exec -f <profile> <original command>
    originalArgs := cmd.Args
    cmd.Path = "/usr/bin/sandbox-exec"
    cmd.Args = append([]string{"sandbox-exec", "-f", profilePath}, originalArgs...)
    return nil
}
```

#### Fallback (unsupported platforms)

```go
// internal/sandbox/sandbox_other.go
func Apply(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
    return fmt.Errorf("sandbox not available on this platform")
}
```

#### Integration

In `cmd/daemon.go`, before `pluginCmd.Start()`:

```go
sandboxPaths := sandbox.SandboxPaths{
    ShimDir:     shim,
    NodeModules: store.NodeModulesPath(),
    NodeRunner:  nodeRunner,
}
if err := sandbox.Apply(pluginCmd, manifest, sandboxPaths); err != nil {
    log.Printf("warning: sandbox not available: %v", err)
    // Continue without sandbox — env filtering still protects credentials
}
```

Fail-open on sandbox setup failure preserves backwards compatibility. The env filtering layer still blocks credential leakage even without OS sandbox.

**Escape hatch**: `c2c connect --no-sandbox <name>` skips sandbox for debugging. Also supported in manifest: `sandbox: false` to permanently disable for a specific plugin.

### Layer 2: Shim Auth

Configurable sender allowlist in `~/.c2c/storage/<name>/config.json`:

```json
{
    "authorized_senders": ["wxid_friend1", "wxid_friend2"],
    "channels": { ... }
}
```

Auth helper changes in `shim/node_modules/@openclaw/plugin-sdk/index.js`:

```javascript
const authorizedSenders = globalConfig.authorized_senders || null;

function resolveSenderCommandAuthorizationWithRuntime(runtime, ctx) {
    if (!authorizedSenders) return { authorized: true }; // no list = allow all
    const senderId = ctx?.senderId || ctx?.from;
    if (authorizedSenders.includes(senderId)) return { authorized: true };
    sendLog(pluginSource, "warn", `Unauthorized sender: ${senderId}`);
    return { authorized: false, reason: "Sender not in authorized_senders list" };
}
```

Default `null` = allow all. No breaking change. Users opt in to sender restriction.

**Config loading**: `c2c-shim.js` already loads `config.json` via `loadConfig()` and passes it to the fake SDK via `setGlobalConfig(config)`. The `authorized_senders` field is read from this existing config object — no new loading mechanism needed.

---

## Theme D: Interface & Test Gaps

### 4a. Structured Skill Arguments (#33)

Change `handleSkill` to accept JSON arrays (preferred) with space-split fallback:

```go
func handleSkill(ctx context.Context, manifest *parser.PluginManifest, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
    var args []string
    argsRaw := request.Params.Arguments["args"]
    switch v := argsRaw.(type) {
    case []interface{}:
        for _, item := range v {
            args = append(args, fmt.Sprint(item))
        }
    case string:
        if v != "" {
            if err := json.Unmarshal([]byte(v), &args); err != nil {
                args = strings.Fields(v) // fallback: backwards compatible
            }
        }
    }
    result, err := runSkillFn(ctx, manifest, args, 30*time.Second)
    // ...
}
```

MCP clients can send `["--query", "hello world"]` as JSON array. Old clients sending `"--query hello world"` still work.

### 4b. cmd/ Package Tests (#19)

**Extract testable logic** from `cmd/` into `internal/installer/`:
- `DerivePluginName(source string) string`
- `ValidatePluginType(pType string) error`

**Cobra command tests** (`cmd/cmd_test.go`):
- `TestInstallCmd_InvalidType` — verify error on bad `--type`
- `TestInstallCmd_InvalidName` — verify path traversal rejection
- `TestRunCmd_MissingPlugin` — verify error on nonexistent plugin
- `TestConnectCmd_NotAConnector` — verify type mismatch error

**Integration tests** (`cmd/integration_test.go`, build-tagged):
- `TestDaemonLifecycle` — start, ping, invoke tool, shutdown, verify cleanup
- `TestSkillExecution` — install fixture, run, verify output

### 4c. Shim Tests (#64)

Test directory: `shim/test/`
Runner: `node:test` (built-in, zero deps)

Test files:
- `shim/test/sdk.test.js` — fake SDK unit tests (NDJSON bridge, pending requests, auth, media)
- `shim/test/shim.test.js` — c2c-shim integration tests (startup, discovery, invoke_tool)
- `shim/test/fixtures/` — mock plugin modules for testing

CI: `cd shim && node --test test/*.test.js`

### 4d. Property-Based / Fuzz Tests

Native Go fuzz targets:

- `internal/protocol/codec_fuzz_test.go` — NDJSON decode should not panic on arbitrary input
- `internal/executor/environment_fuzz_test.go` — `isSafeEnvVar` should not panic
- `internal/parser/manifest_fuzz_test.go` — manifest parsing should not panic on arbitrary YAML
- `internal/paths/paths_fuzz_test.go` — `ValidateName` should not panic and should correctly reject traversal patterns

---

## Implementation Order

Dependencies between themes determine build order:

```
Phase 1: internal/store (Theme A)
  └── Enables: local packages, pinned tsx, verified installs
Phase 2: Socket protocol (Theme B)
  └── Depends on: nothing (parallel with Phase 1)
Phase 3: internal/sandbox (Theme C, Layer 1)
  └── Depends on: Phase 1 (needs local node_modules paths)
Phase 4: Shim auth (Theme C, Layer 2)
  └── Depends on: nothing (parallel with Phase 3)
Phase 5: Structured args + extracted logic (Theme D, 4a + 4b)
  └── Depends on: nothing (parallel)
Phase 6: Tests (Theme D, 4b + 4c + 4d)
  └── Depends on: Phases 1-5 (tests exercise new features)
```

Phases 1+2 can run in parallel. Phases 3+4+5 can run in parallel after Phase 1. Phase 6 runs last.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/store/store.go` | Local package management |
| `internal/store/store_test.go` | Store unit tests |
| `internal/store/tsx.go` | Shared tsx management |
| `internal/sandbox/sandbox.go` | Common types and SandboxPaths |
| `internal/sandbox/sandbox_linux.go` | Landlock + seccomp implementation |
| `internal/sandbox/sandbox_darwin.go` | sandbox-exec implementation |
| `internal/sandbox/sandbox_other.go` | Fallback (no-op with warning) |
| `internal/sandbox/sandbox_test.go` | Sandbox unit tests |
| `internal/installer/installer.go` | Extracted install logic from cmd/ |
| `internal/installer/installer_test.go` | Installer unit tests |
| `cmd/cmd_test.go` | Cobra command tests |
| `cmd/integration_test.go` | Build-tagged integration tests |
| `shim/test/sdk.test.js` | Fake SDK unit tests |
| `shim/test/shim.test.js` | Shim integration tests |
| `shim/test/fixtures/mock-plugin.js` | Test fixture |
| `internal/protocol/codec_fuzz_test.go` | NDJSON fuzz target |
| `internal/executor/environment_fuzz_test.go` | Env filter fuzz target |
| `internal/parser/manifest_fuzz_test.go` | Manifest fuzz target |

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/daemon.go` | Remove EnsurePluginInstalled, use local store, add ping/pong/shutdown handlers, integrate sandbox |
| `cmd/install.go` | Use internal/store for local installs, write resolved_version + integrity |
| `internal/executor/daemon.go` | Add waitForReady after Start, socket-based StopConnector |
| `internal/executor/permission.go` | Integrate with sandbox.Apply |
| `internal/mcp/server.go` | Structured args parsing in handleSkill |
| `internal/executor/runner.go` | Replace npx -y with local store execution, remove preInstallPackage |
| `internal/nodeutil/nodeutil.go` | Remove EnsurePluginInstalled, simplify to resolution-only |
| `internal/protocol/messages.go` | Add Ping/Pong/Shutdown message types |
| `go.mod` | Add `github.com/landlock-lsm/go-landlock` dependency |
| `internal/parser/types.go` | Add ResolvedVersion and Integrity fields to manifest |
| `shim/node_modules/@openclaw/plugin-sdk/index.js` | Auth allowlist implementation |
| `.github/workflows/ci.yml` | Add shim test step |
| `Makefile` | Add shim-test target |
