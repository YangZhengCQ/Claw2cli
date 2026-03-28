# Claw2Cli Implementation Guide

> Last updated: 2026-03-28

## Table of Contents

- [1. Project Structure](#1-project-structure)
- [2. Build and Test](#2-build-and-test)
- [3. Plugin Install Flow](#3-plugin-install-flow)
- [4. Connector Lifecycle](#4-connector-lifecycle)
- [5. Shim Layer Details](#5-shim-layer-details)
- [6. NDJSON Protocol](#6-ndjson-protocol)
- [7. Capability Discovery](#7-capability-discovery)
- [8. MCP Integration](#8-mcp-integration)
- [9. Key Functions Reference](#9-key-functions-reference)
- [10. Test Coverage](#10-test-coverage)

## 1. Project Structure

```
Claw2Cli/
├── main.go                          # Entry point
├── cmd/                             # Cobra CLI commands
│   ├── root.go                      # Root command + subcommand registration
│   ├── install.go                   # c2c install — plugin installation with pre-flight checks
│   ├── connect.go                   # c2c connect — background (default) / -f foreground
│   ├── daemon.go                    # Hidden _daemon subcommand + shim process management
│   ├── run.go                       # c2c run — skill execution
│   ├── attach.go                    # c2c attach — UDS stream viewer
│   ├── echo.go                      # c2c echo — test consumer (auto-reply)
│   ├── call.go                      # c2c call — generic RPC tool invocation
│   ├── stop.go                      # c2c stop — graceful shutdown
│   ├── status.go                    # c2c status — connector status table
│   ├── list.go                      # c2c list — installed plugins
│   ├── info.go                      # c2c info — plugin details + discovered tools
│   ├── logs.go                      # c2c logs — tail daemon logs
│   └── mcp.go                       # c2c mcp serve — MCP server
├── internal/
│   ├── store/
│   │   ├── store.go                 # Local npm package management (per-plugin node_modules)
│   │   └── tsx.go                   # tsx binary resolution and installation
│   ├── sandbox/
│   │   ├── sandbox.go               # Platform-agnostic sandbox interface
│   │   ├── sandbox_darwin.go        # macOS sandbox-exec implementation
│   │   ├── sandbox_linux.go         # Linux sandbox (stub — landlock/seccomp planned)
│   │   └── sandbox_other.go         # No-op for unsupported platforms
│   ├── executor/
│   │   ├── runner.go                # Skill subprocess runner (tsx + local store + timeout)
│   │   ├── daemon.go                # Connector lifecycle (start/stop/attach/status)
│   │   ├── permission.go            # Permission guard (pre-exec check)
│   │   ├── environment.go           # Environment variable builder
│   │   └── deps.go                  # Test dependency injection
│   ├── nodeutil/
│   │   └── nodeutil.go              # Node.js/npm helpers (resolve runner, install packages)
│   ├── registry/
│   │   └── registry.go              # Tool schema cache (sync.Map, per-connector)
│   ├── mcp/
│   │   ├── server.go                # MCP Server (stdio JSON-RPC) + dynamic tool registration
│   │   ├── dynamic.go               # UDS-based tool discovery + invocation helpers
│   │   ├── converter.go             # Manifest → MCP Tool converter
│   │   ├── filter.go                # Plugin filter (--skills/--connectors)
│   │   └── deps.go                  # Test dependency injection
│   ├── parser/
│   │   ├── types.go                 # PluginManifest, SkillMetadata, PluginType
│   │   ├── manifest.go              # manifest.yaml parser + plugin discovery
│   │   └── skillmd.go               # SKILL.md parser (YAML frontmatter)
│   ├── paths/paths.go               # Directory layout helpers (~/.c2c/*)
│   └── protocol/
│       ├── messages.go              # NDJSON message types + constructors
│       └── codec.go                 # NDJSON encoder/decoder
├── shim/
│   ├── c2c-shim.js                  # Shim entry point (loads plugin, manages gateway)
│   └── node_modules/
│       ├── @openclaw/plugin-sdk/    # Fake SDK (main shim implementation)
│       └── openclaw/plugin-sdk/     # Re-export for alternate import path
└── testdata/                        # Test fixtures
```

## 2. Build and Test

```bash
make build          # go build -o c2c .
make test           # go test -race ./...
make lint           # go vet ./...
make install        # go install .
make clean          # rm -f c2c coverage.out
```

## 3. Plugin Install Flow

`cmd/install.go` — `installPlugin()`:

```
1. checkNodeNpm()         — verify node and npm on PATH
2. checkShimFiles()       — verify c2c-shim.js and fake SDK exist
3. paths.EnsureDirs()     — create ~/.c2c/{plugins,storage,sockets,pids}
4. derivePluginName()     — "@tencent-weixin/openclaw-weixin-cli@latest" → "wechat"
5. nodeutil.GetNpmChecksum() — fetch SHA-512 integrity hash from npm registry
6. Write manifest.yaml    — source, type, permissions, checksum
7. paths.EnsureStorageDir — create ~/.c2c/storage/<name>/ with 0700
8. store.Install()        — npm install --ignore-scripts locally (both CLI wrapper and runtime package)
```

**Security:** All `npm install` commands use `--ignore-scripts` to prevent supply-chain attacks
via malicious `postinstall` scripts. Conflicting transitive deps (`openclaw`, `clawdbot`, `@mariozechner`)
are auto-cleaned after install since the shim replaces them at runtime.

**Flags:**
- `--skip-verify` — skip npm integrity verification (use when registry is unreachable)
- `--type connector|skill` — override auto-detected plugin type

**Retry behavior:** If local `npm install` fails during `c2c install`, the error is non-fatal
(warning printed). The install is retried automatically on the first `c2c connect`.

**CLI vs runtime package resolution** (`resolvePluginPackage()`):
- Strip version suffix: `@scope/name@version` → `@scope/name`
- Strip `-cli` suffix: `@scope/name-cli` → `@scope/name`
- Both packages are installed locally to `~/.c2c/plugins/<name>/node_modules/`

## 4. Connector Lifecycle

### Start (background: `executor.StartConnector`)
1. Check permissions (network required for connectors)
2. Check if already running (PID file + process alive check)
3. Find own binary path → `exec.Command(self, "_daemon", name)`
4. Set `SysProcAttr.Setsid = true` (detach from terminal)
5. Redirect stdout/stderr to `~/.c2c/logs/<name>.log`
6. `cmd.Start()` → write PID file + metadata JSON
7. `cmd.Process.Release()` (don't wait for child)

### Start (foreground: `connect.go` → `runDaemon()`)
1. `runDaemon()` called directly (no detach, no PID file)
2. `isForeground = true` → stderr passthrough, terminal output
3. UDS listener still created (echo/attach can connect)

### The `runDaemon()` core (`cmd/daemon.go`):
1. Load manifest from `~/.c2c/plugins/<name>/manifest.yaml`
2. Locate shim: `shimDir()` → `shim/c2c-shim.js`
3. Build `NODE_PATH`: `shim/node_modules` + global npm root
4. `ensurePluginInstalled()` — install CLI + runtime packages if missing
5. `resolveNodeRunner()` — prefer tsx (auto-install if needed), fallback to node
6. Start subprocess: `tsx c2c-shim.js <name>` with env vars
7. Pipe stdout → parse NDJSON → broadcast to UDS clients (+ terminal in foreground)
8. Pipe stderr → broadcast as log (or passthrough in foreground)
9. Listen on UDS `~/.c2c/sockets/<name>.sock`
10. Accept UDS clients → forward commands/responses to shim stdin
11. Wait for SIGTERM/SIGINT or process exit
12. Graceful shutdown: SIGTERM → 3s timeout → SIGKILL

### Stop (`executor.StopConnector`)
1. Read PID from file
2. Send `SIGTERM`
3. Wait up to 5s for exit
4. If still alive: `SIGKILL`
5. Cleanup: remove PID file, metadata, socket

### Attach (`executor.AttachConnector`)
1. Try PID-based status check (background mode)
2. Fallback: direct socket connection (foreground mode — no PID file)
3. Return `net.Conn` for bidirectional NDJSON streaming

## 5. Shim Layer Details

### Entry point: `shim/c2c-shim.js`
1. Load config from `C2C_STORAGE_DIR/config.json`
2. Resolve plugin package: strip `-cli` suffix from `C2C_PLUGIN_SOURCE`
3. Load plugin: `await import(pkg)` (ESM) → fallback `require(pkg)` (CJS)
4. Call `plugin.register(api)` with `PluginApiShim` instance
5. Get registered channel → list accounts → start gateway per account
6. If no accounts: trigger QR login flow

### Fake SDK: `shim/node_modules/@openclaw/plugin-sdk/index.js`

**NDJSON bridge:**
- `sendMessage(msg)` → `process.stdout.write(JSON.stringify(msg) + "\n")`
- `sendCommand(source, action, payload)` → write command, return Promise (matched by `id`)
- Stdin readline → match responses by `id`, forward inbound commands

**EPIPE handling:**
```javascript
process.stdout.on("error", (err) => {
  if (err.code === "EPIPE" || err.code === "ERR_STREAM_DESTROYED") {
    stdoutClosed = true;   // silently stop writing
  }
});
```

**Stdin close handling (2026-03-28):**
When the Go daemon crashes or shuts down, `rl.on("close")` rejects all pending requests
immediately instead of waiting for their 5-minute timeouts. This prevents memory leaks
from accumulated unresolved Promises.

**Stdin unref (2026-03-28):**
The readline on `process.stdin` keeps the Node.js event loop alive indefinitely. In production
this is fine (gateway loop keeps process alive), but in tests it prevents `node --test` from
exiting. Fix: `process.stdin._handle.unref()` after readline creation — allows the process to
exit when no other work is pending.

**Error separation:** The `rl.on("line")` catch block distinguishes `SyntaxError` (malformed
JSON — silently ignored) from other errors (handler bugs — logged via `sendLog`).

**Key shim function — `dispatchReplyFromConfig`:**
1. `sendEvent("message.received", {from, body, ...})` — notify Go side
2. `sendCommand("get_reply", {from, body, ...})` — wait for reply (5min timeout)
3. If reply received: `dispatcher.dispatch({text: reply.text})` — send back via plugin

## 6. NDJSON Protocol

Defined in `internal/protocol/messages.go`:

| Type | Direction | Purpose |
|------|-----------|---------|
| `event` | shim → daemon → clients | Server push (message received, status change) |
| `command` | shim → daemon → clients, or client → daemon → shim | Request (get_reply, send_text) |
| `response` | client → daemon → shim | Reply to a command (matched by `id`) |
| `error` | any direction | Unrecoverable error notification |
| `log` | shim → daemon → clients | Debug logging (filterable by level/source) |
| `discovery` | shim → daemon (cached) → clients | Tool schema declaration (MCP Tool Schema format) |

All messages carry `source` field for multi-connector routing.

`command`/`response` use `id` field for request-response correlation.

## 7. Capability Discovery

### Data Flow

```
Plugin registers channel (outbound.sendText, outbound.sendMedia, etc.)
    ↓
Shim: emitDiscovery() introspects channel → builds MCP Tool Schemas
    ↓
Shim emits: {"type":"discovery","source":"wechat","payload":{"tools":[...],"agentHints":[...]}}
    ↓
Daemon: caches tools in toolRegistry (sync.Map, key=connector name)
    ↓
Consumers read via UDS:
  ├─ c2c call --list-tools    → sends "list_tools" command → daemon responds from cache
  ├─ c2c call <tool> [args]   → sends "invoke_tool" command → daemon forwards to shim
  ├─ c2c info <plugin>        → queries list_tools if connector is running
  └─ c2c mcp serve            → queries list_tools at startup → registers MCP tools
```

### Shim: `emitDiscovery()` (`shim/node_modules/@openclaw/plugin-sdk/index.js`)

Called after `PluginApiShim.registerChannel()`. Introspects:
- `channel.outbound.sendText` → generates `<source>_send_text` tool
- `channel.outbound.sendMedia` → generates `<source>_send_media` tool
- `channel.agentPrompt.messageToolHints()` → extracted as `agentHints[]` (separate from tool descriptions)

Tool descriptions are concise for CLI display. Agent hints are a separate array in the discovery payload for MCP to optionally append.

### Shim: `handleInvokeTool()` (bridge functions)

When shim receives `{"action":"invoke_tool","payload":{"tool":"wechat_send_text","args":{...}}}`:
1. Match tool name suffix (`_send_text`, `_send_media`)
2. Call the corresponding plugin method (`outbound.sendText`, `outbound.sendMedia`)
3. Encapsulate internals: context tokens, CDN upload, session guards — caller never sees these
4. Return result as `response` or error

### Daemon: `toolRegistry` (`cmd/daemon.go`)

```go
var toolRegistry sync.Map  // connector name → []protocol.ToolSchema

// On "discovery" message from shim: toolRegistry.Store(name, tools)
// On connector stop/crash: toolRegistry.Delete(name)
// On "list_tools" from UDS client: respond from cache (no shim round-trip)
```

Exported functions: `GetDiscoveredTools(name)`, `GetAllDiscoveredTools()`

### CLI: `c2c call` (`cmd/call.go`)

```bash
c2c call <connector> --list-tools              # query tool schemas
c2c call <connector> <tool> '{"key":"value"}'   # invoke tool, wait for response
c2c call <connector> <tool> --timeout 60        # custom timeout (default 30s)
```

Connects to UDS, sends command, waits for matching response by `id`.

## 8. MCP Integration

`internal/mcp/server.go` — `Serve()`:
1. Scan installed plugins via `parser.ListPlugins()`
2. Filter by `--skills`/`--connectors` flags
3. Convert each manifest to an MCP tool via `ManifestToTool()`
4. Register tools with `mcp-go` server
5. Serve over stdio (JSON-RPC)

Skill tools: receive `args` → resolve from local store → execute via tsx subprocess
Connector tools (static): receive `action` string → dispatch to StartConnector/StopConnector/UDS forward
Connector tools (dynamic): `registerDynamicTools()` queries each running connector via UDS `list_tools`, registers discovered tools with handlers that forward `invoke_tool` via `InvokeTool()`

**Verified end-to-end:** MCP `tools/call("wechat_send_text", {to, text})` → UDS → daemon → shim bridge (auto-resolve accountId, get contextToken) → `sendMessageWeixin` → message delivered.

**Plugin compatibility matrix:**

| Plugin | Tools | Type | Notes |
|--------|-------|------|-------|
| WeChat (`@tencent-weixin/openclaw-weixin-cli`) | 2 | Channel | send_text, send_media. Full E2E verified (sandbox + local store, 2026-03-28). |
| Feishu (`@larksuite/openclaw-lark`) | 27 | Channel + OAPI | calendar, task, bitable, im, chat, doc, wiki, drive, search, OAuth |
| QQ Bot (`@tencent-connect/openclaw-qqbot`) | 3 | Channel + OAPI | send_text, send_media, remind |
| WeCom (`@wecom/wecom-openclaw-plugin`) | 3 | Channel + MCP | send_text, send_media, wecom_mcp (HTTP MCP bridge) |
| Web Search (`@ollama/openclaw-web-search`) | 2 | Skill-only | ollama_web_search, ollama_web_fetch. No channel, stays alive for tool calls. |
| Tavily (`openclaw-tavily`) | 5 | Skill-only | Idle without API key. Loads successfully. |
| DingTalk (`@dingtalk-real-ai/dingtalk-connector`) | — | — | Blocked by upstream dep bug (not c2c) |

**E2E verification (2026-03-28):** WeChat connector tested with all hardening features active:

| Feature | Verification |
|---------|-------------|
| Local package store | Loaded from `~/.c2c/plugins/wechat/node_modules/`, no global npm at runtime |
| Sandbox | `sandbox-exec` applied (no "sandbox not available" warning) |
| Integrity check | `c2c update wechat` detected checksum change and re-installed |
| Tool discovery | 2 tools registered (send_text, send_media) + 5 agent hints |
| Account startup | Existing account `3ffc51ae8b27_im_bot` started, WeChat long-poll connected |
| Graceful shutdown | SIGTERM → 9s grace → SIGKILL (WeChat long-poll holds the process past grace period) |

## 9. Key Functions Reference

| Function | File | Purpose |
|----------|------|---------|
| `ResolvePluginPackage(source)` | `internal/nodeutil/nodeutil.go` | Strip version + `-cli` suffix → runtime package name |
| `store.New(name)` | `internal/store/store.go` | Create local package store for a plugin |
| `store.Install(source)` | `internal/store/store.go` | Install packages locally to plugin's node_modules |
| `store.CleanupReplacedPackages()` | `internal/store/store.go` | Remove openclaw/clawdbot/pi-ai from local deps |
| `store.ResolveTsx()` | `internal/store/tsx.go` | Return local or global tsx binary path |
| `sandbox.Apply(cmd, manifest, paths)` | `internal/sandbox/sandbox.go` | Apply OS-level sandbox to plugin subprocess |
| `registry.Store(name, tools)` | `internal/registry/registry.go` | Cache tool schemas for a connector |
| `registry.Get(name)` | `internal/registry/registry.go` | Get cached tool schemas for a connector |
| `registry.GetAll()` | `internal/registry/registry.go` | Get tools from all active connectors |
| `registry.Delete(name)` | `internal/registry/registry.go` | Evict tools on stop/crash |
| `DiscoverTools(name)` | `internal/mcp/dynamic.go` | Query connector via UDS for tool schemas |
| `InvokeTool(name, tool, args)` | `internal/mcp/dynamic.go` | Send invoke_tool via UDS, wait for result |
| `ValidateName(name)` | `internal/paths/paths.go` | Reject path-traversal in plugin names |
| `BuildEnv(manifest)` | `internal/executor/environment.go` | Build filtered env vars for plugin subprocess |
| `CheckPermissions(manifest)` | `internal/executor/permission.go` | Pre-exec permission guard |
| `GetNpmChecksum(source)` | `internal/nodeutil/nodeutil.go` | Fetch package integrity hash from npm registry (canonical — used by install + store) |
| `sandbox.Cleanup()` | `internal/sandbox/sandbox.go` | Remove temp sandbox profile after command exits |
| `rotateLogIfNeeded(path)` | `internal/executor/daemon.go` | Rotate log file if >10MB (keeps one `.1` backup) |
| `checkNodeNpm()` | `cmd/install.go` | Pre-flight: verify node/npm on PATH |
| `checkShimFiles()` | `cmd/install.go` | Pre-flight: verify shim files exist |
| `shimDir()` | `cmd/daemon.go` | Locate shim directory relative to binary |
| `emitDiscovery()` | `shim/.../index.js` | Introspect channel → emit MCP Tool Schemas |
| `handleInvokeTool(msg)` | `shim/.../index.js` | Dispatch invoke_tool to plugin outbound methods |

## 10. Test Coverage

As of 2026-03-28 (post grill report fixing plan — 22 of 25 items completed):

| Package | Coverage | Notes |
|---------|----------|-------|
| internal/registry | 94.1% | |
| internal/parser | 94.0% | |
| internal/executor | 88.0% | +0.4 — log rotation tests |
| internal/protocol | 86.7% | |
| internal/mcp | 80.0% | +0.4 — goroutine leak fix + read deadline |
| internal/nodeutil | 71.2% | +39.7 — GetNpmChecksum, ResolveNodeRunner, EnsurePluginInstalled |
| internal/paths | 66.7% | +3.8 — initBaseDir fail-fast tests |
| internal/store | 64.4% | +15.5 — Verify tests (getIntegrity delegated to nodeutil) |
| internal/sandbox | 58.1% | +0.2 — Cleanup + writeTempProfile tests |
| **Total** | **80.4%** | **+5.9 from 74.5%** |

**JS shim tests:** 4 test files, 39 tests total (auth 25, sdk 2, subpath 4, shim-fixes 8).

`cmd/` and `main.go` are excluded from coverage (CLI integration layer).

**Remaining uncoverable paths:**
- `protocol/Encode`: `json.Marshal` error path unreachable with `Message` struct
- `executor/isProcessRunning`: `os.FindProcess` error branch unreachable on Unix
- `mcp/Serve`: blocks on stdio — needs integration test
- `nodeutil/ResolveNodeRunner` interactive path: requires terminal simulation

**Checksum consolidation (2026-03-28):**
- `cmd/install.go:getNpmChecksum` → removed, calls `nodeutil.GetNpmChecksum`
- `internal/store/store.go:getIntegrity` → one-liner delegating to `nodeutil.GetNpmChecksum`
- `nodeutil.GetNpmChecksum` is the canonical single source of truth

**Tool-discovery consolidation (2026-03-28):**
- `cmd/info.go:queryDiscoveredTools` → 5-line wrapper calling `mcp.DiscoverTools`
- `mcp.DiscoverTools` is the canonical implementation
