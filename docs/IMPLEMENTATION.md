# Claw2Cli Implementation Guide

> Last updated: 2026-03-23

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
│   ├── connect.go                   # c2c connect — foreground/background mode switch
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
│   ├── config/config.go             # Global config loading (~/.c2c/config.yaml)
│   ├── executor/
│   │   ├── runner.go                # Skill subprocess runner (npx + timeout)
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
5. getNpmChecksum()       — fetch SHA-512 integrity hash from npm registry
6. Write manifest.yaml    — source, type, permissions, checksum
7. paths.EnsureStorageDir — create ~/.c2c/storage/<name>/ with 0700
8. preInstallPackage()    — npm install -g (both CLI wrapper and runtime package)
```

**CLI vs runtime package resolution** (`resolvePluginPackage()`):
- Strip version suffix: `@scope/name@version` → `@scope/name`
- Strip `-cli` suffix: `@scope/name-cli` → `@scope/name`
- Both packages are installed globally for fast `connect` startup

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

Skill tools: receive `args` string → split → pass to npx subprocess
Connector tools (static): receive `action` string → dispatch to StartConnector/StopConnector/UDS forward
Connector tools (dynamic): `registerDynamicTools()` queries each running connector via UDS `list_tools`, registers discovered tools with handlers that forward `invoke_tool` via `InvokeTool()`

**Verified end-to-end:** MCP `tools/call("wechat_send_text", {to, text})` → UDS → daemon → shim bridge (auto-resolve accountId, get contextToken) → `sendMessageWeixin` → message delivered.

**Plugin compatibility matrix:**

| Plugin | Tools discovered | Channel + OAPI |
|--------|-----------------|----------------|
| WeChat (`@tencent-weixin/openclaw-weixin-cli`) | 2 | send_text, send_media |
| Feishu (`@larksuite/openclaw-lark`) | 27 | send_text, send_media + calendar, task, bitable, im, chat, doc, wiki, drive, search, OAuth |
| QQ Bot (`@tencent-connect/openclaw-qqbot`) | 3 | send_text, send_media + remind |
| DingTalk (`@dingtalk-real-ai/dingtalk-connector`) | — | Blocked by upstream dep bug (not c2c) |

## 9. Key Functions Reference

| Function | File | Purpose |
|----------|------|---------|
| `ResolvePluginPackage(source)` | `internal/nodeutil/nodeutil.go` | Strip version + `-cli` suffix → runtime package name |
| `ResolveNodeRunner()` | `internal/nodeutil/nodeutil.go` | Return `tsx` path (auto-install if needed), fallback `node` |
| `ResolveGlobalNodeModules()` | `internal/nodeutil/nodeutil.go` | Find global npm node_modules directory |
| `EnsurePluginInstalled(source)` | `internal/nodeutil/nodeutil.go` | Install CLI + runtime packages globally |
| `registry.Store(name, tools)` | `internal/registry/registry.go` | Cache tool schemas for a connector |
| `registry.Get(name)` | `internal/registry/registry.go` | Get cached tool schemas for a connector |
| `registry.GetAll()` | `internal/registry/registry.go` | Get tools from all active connectors |
| `registry.Delete(name)` | `internal/registry/registry.go` | Evict tools on stop/crash |
| `DiscoverTools(name)` | `internal/mcp/dynamic.go` | Query connector via UDS for tool schemas |
| `InvokeTool(name, tool, args)` | `internal/mcp/dynamic.go` | Send invoke_tool via UDS, wait for result |
| `ValidateName(name)` | `internal/paths/paths.go` | Reject path-traversal in plugin names |
| `BuildEnv(manifest)` | `internal/executor/environment.go` | Build filtered env vars for plugin subprocess |
| `CheckPermissions(manifest)` | `internal/executor/permission.go` | Pre-exec permission guard |
| `checkNodeNpm()` | `cmd/install.go` | Pre-flight: verify node/npm on PATH |
| `checkShimFiles()` | `cmd/install.go` | Pre-flight: verify shim files exist |
| `shimDir()` | `cmd/daemon.go` | Locate shim directory relative to binary |
| `emitDiscovery()` | `shim/.../index.js` | Introspect channel → emit MCP Tool Schemas |
| `handleInvokeTool(msg)` | `shim/.../index.js` | Dispatch invoke_tool to plugin outbound methods |

## 10. Test Coverage

As of 2026-03-23 (Phase 1):

| Package | Coverage |
|---------|----------|
| internal/config | 100.0% |
| internal/registry | 100.0% |
| internal/parser | 98.2% |
| internal/protocol | 95.8% |
| internal/paths | 95.7% |
| internal/executor | 91.4% |
| internal/mcp | 43.1% |
| internal/nodeutil | 21.1% |

`cmd/` and `main.go` are excluded from coverage (CLI integration layer).

Unreachable gaps:
- `mcp/Serve`: calls `server.ServeStdio` which blocks on stdio
- `paths/init`: `os.UserHomeDir()` error fallback unreachable in tests
- `protocol/Encode`: `json.Marshal` error path unreachable with `Message` struct
- `executor/isProcessRunning`: `os.FindProcess` error branch unreachable on Unix
