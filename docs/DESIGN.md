# Claw2Cli Design Document

> Last updated: 2026-03-23 (added Capability Discovery as core architecture)

## Table of Contents

- [1. Project Vision](#1-project-vision)
- [2. Upstream Ecosystem: OpenClaw](#2-upstream-ecosystem-openclaw)
- [3. Plugin Model](#3-plugin-model)
- [4. Architecture](#4-architecture)
  - [4.1 Tech Stack](#41-tech-stack)
  - [4.2 Thin Compatibility Layer](#42-thin-compatibility-layer)
  - [4.3 JS-Go Bridge](#43-js-go-bridge)
  - [4.4 Daemon Model](#44-daemon-model)
  - [4.5 System Overview](#45-system-overview)
  - [4.6 Security Model](#46-security-model)
  - [4.7 Storage Isolation](#47-storage-isolation)
  - [4.8 MCP Server](#48-mcp-server)
  - [4.9 Plugin Runtime Shim](#49-plugin-runtime-shim)
  - [4.10 Capability Discovery](#410-capability-discovery)
- [5. Target Platforms](#5-target-platforms)
- [6. Roadmap](#6-roadmap)
- [7. Design Decisions](#7-design-decisions)
- [8. Open Questions](#8-open-questions)

## 1. Project Vision

Claw2Cli is a **Go CLI compatibility layer** that extracts high-quality plugins from the OpenClaw ecosystem and exposes them as standard CLI tools.

**Core insight:** Production developers won't run OpenClaw (too heavy, too insecure), but major companies like Tencent, ByteDance, and Kimi are publishing valuable plugins for it. Claw2Cli captures this ecosystem value and delivers it to any shell consumer — Claude Code, Python scripts, CI/CD pipelines, manual use.

**What it is NOT:**
- Not a fork or trimmed version of OpenClaw
- Not embedding a Node.js runtime
- Not rewriting plugin logic

## 2. Upstream Ecosystem: OpenClaw

| Property | Value |
|----------|-------|
| Repository | github.com/openclaw/openclaw |
| Scale | 330k stars, 78 extensions, 52 skills |
| Stack | TypeScript / Node.js / pnpm |
| Plugin protocol | SKILL.md (YAML frontmatter + Markdown body) |
| Security | Docker sandbox (group/channel sessions) |
| License | MIT |

## 3. Plugin Model

### 3.1 Skills (Stateless)

- One-shot subprocess: start → process args → return output → exit
- Invocation: `c2c run <skill> [args]`
- Permissions: restricted (network + temp files only)

### 3.2 Connectors (Stateful)

- Long-lived daemon maintaining persistent connections (WeChat, Feishu, etc.)
- Invocation: `c2c connect <connector>` (foreground by default, `-b` for background)
- Permissions: controlled elevated (network + persistent storage + credentials)

| Dimension | Skill | Connector |
|-----------|-------|-----------|
| Network | Restricted | Full |
| Filesystem | Temp directory | Dedicated data directory |
| Persistent state | No | Yes (session persistence) |
| Credential storage | No | Yes |
| Process model | One-shot subprocess | Daemon |

## 4. Architecture

### 4.1 Tech Stack

**Language:** Go 1.22+ (developed with Go 1.26.1)
**Binary size:** ~8MB (single binary, no external dependencies)

Rationale:
- **Single binary distribution** — no Node.js install required (plugin deps fetched via npx on demand)
- **Goroutines** — natural fit for concurrent long-lived connections
- **Cross-platform** — macOS (darwin/arm64, darwin/amd64) + Linux (linux/amd64, linux/arm64)

**Core dependencies:**

| Library | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Config management (YAML) |
| `gopkg.in/yaml.v3` | YAML parsing |
| `github.com/mark3labs/mcp-go` | MCP Server SDK |

### 4.2 Thin Compatibility Layer

Go calls plugin subprocesses via `os/exec`, running OpenClaw plugins natively:

```
c2c run google-search --query "AI news"
    │
    ▼
Parse SKILL.md → build npx command → execute subprocess → collect stdout/stderr → format output
```

Benefits:
- **Isolation:** plugin crash only kills the subprocess, not the Go process
- **Version control:** parse version from SKILL.md, dynamically call `npx @package@version`
- **Zero JS deps:** Go binary doesn't need Node.js; plugin deps fetched via npx

### 4.3 JS-Go Bridge

**Skills — stdin/stdout pipe (simple):**
```
Go → os/exec → npx skill-plugin --json → stdout → Go parses JSON result
```

**Connectors — stdin/stdout + Unix Domain Socket (full-duplex):**
```
Go daemon
    │
    ├─ stdin/stdout pipe ← control signals (start, stop, status)
    │
    └─ Unix Domain Socket ← data stream (message send/receive, full-duplex)
         Path: ~/.c2c/sockets/<connector-name>.sock
```

Why UDS over pure pipe:
- Pipe is half-duplex; UDS is full-duplex
- UDS supports multiple concurrent consumers (CLI viewer + MCP forwarder in parallel)
- UDS has a filesystem path, enabling `c2c attach` to reconnect to an existing daemon

**Protocol: NDJSON (Newline Delimited JSON)**

Five message types, all carrying a `source` field for multi-connector routing:

```jsonl
{"type":"event","source":"wechat","topic":"message.received","payload":{...},"ts":1711100000}
{"type":"command","source":"wechat","action":"send_message","payload":{...},"id":"req-001"}
{"type":"response","source":"wechat","payload":{...},"id":"req-001"}
{"type":"error","source":"feishu","code":"AUTH_FAILED","message":"...","ts":1711100000}
{"type":"log","source":"wechat","level":"info","message":"heartbeat ok","ts":1711100000}
```

**Environment variables injected into all plugin subprocesses:**

| Variable | Description |
|----------|-------------|
| `C2C_PLUGIN_NAME` | Plugin name |
| `C2C_PLUGIN_TYPE` | `skill` or `connector` |
| `C2C_STORAGE_DIR` | Plugin-specific storage path (`~/.c2c/storage/<name>`) |
| `C2C_BASE_DIR` | c2c root directory (`~/.c2c`) |
| `C2C_PLUGIN_SOURCE` | Original npm package specifier |
| `NODE_PATH` | `shim/node_modules` + global `node_modules` (module path hijacking) |

### 4.4 Daemon Model

Go doesn't support `fork()`. We use a **hidden subcommand self-reinvocation** pattern:

```
c2c connect wechat          # Foreground (default): direct daemon, QR codes visible
c2c connect wechat -b       # Background: detached via Setsid

Foreground mode:
    Go calls runDaemon() directly
    │
    ├─ Subprocess: tsx c2c-shim.js wechat (ESM + TypeScript)
    ├─ stdout → parse NDJSON, print logs/events to terminal
    ├─ stderr → passthrough to terminal (QR codes, interactive prompts)
    ├─ Ctrl+C → SIGTERM → 3s grace → SIGKILL
    └─ UDS listener (supports parallel attach)

Background mode:
    Go executes `c2c _daemon wechat` (hidden subcommand), detached via Setsid
    │
    ├─ Subprocess: tsx c2c-shim.js wechat
    ├─ stdout/stderr → broadcast as NDJSON
    ├─ UDS listener (~/.c2c/sockets/wechat.sock)
    ├─ PID → ~/.c2c/pids/wechat.pid
    ├─ Metadata → ~/.c2c/pids/wechat.json
    └─ Logs → ~/.c2c/logs/wechat.log
```

Management commands:
- `c2c attach` — connect to UDS (PID-based lookup first, direct socket fallback for foreground mode)
- `c2c status` — scan PID files, check process liveness
- `c2c stop` — SIGTERM → 5s → SIGKILL, cleanup PID/socket/metadata files

### 4.5 System Overview

```
User / Agent / Script
        │
        ▼
┌──────────────────┐
│     c2c CLI       │  ← Go single binary
├──────────────────┤
│  Command Router   │  ← run / connect / list / attach / echo / stop / mcp
├──────┬───────────┤
│ Skill │ Connector │
│Runner │ Manager   │
│(pipe) │ (daemon)  │
├──────┴───────────┤
│   Plugin Shim     │  ← SKILL.md parser + tsx/npx bridge
├──────────────────┤
│ Cap. Discovery    │  ← introspect plugin → surface as tools
├──────────────────┤
│ Permission Guard  │  ← manifest-based access control
├──────────────────┤
│  MCP Server       │  ← JSON-RPC over stdio (dynamic tools from capabilities)
└──────────────────┘
        │
        ▼
  OpenClaw plugins (npm packages, run natively)
```

### 4.6 Security Model

**MVP (Phase 1) — Static defenses:**
- **Package integrity:** SHA-512 checksum recorded on `c2c install`; runtime verification in Phase 2
- **Declarative permissions:** c2c-owned `manifest.yaml` (doesn't modify upstream SKILL.md); checked before `os/exec`
- **Install pre-flight:** `c2c install` validates node/npm availability and shim file integrity before proceeding

**Phase 2 — Runtime sandbox:**
- macOS: `sandbox-exec` (userland, no root)
- Linux: `seccomp-bpf` (lightweight syscall filtering, no root)

### 4.7 Storage Isolation

```
~/.c2c/
  ├── plugins/<name>/manifest.yaml   # Plugin metadata + permission manifest
  ├── storage/<name>/                # Plugin-specific data, 0700 permissions
  ├── sockets/<name>.sock            # UDS (connectors only)
  ├── pids/<name>.pid                # PID file (connectors only)
  ├── pids/<name>.json               # Metadata (connectors only)
  ├── logs/<name>.log                # Daemon logs (connectors only)
  └── config.yaml                    # Global configuration
```

### 4.8 MCP Server

Claw2Cli runs as a standard MCP Server, exposing installed plugins as MCP tools:

```json
{
  "c2c": {
    "command": "c2c",
    "args": ["mcp", "serve"]
  }
}
```

Connector tool actions:

| Action | Behavior |
|--------|----------|
| `start` | Launch daemon via `StartConnector` |
| `stop` | Shut down via `StopConnector` |
| `status` | Return JSON status via `GetConnectorStatus` |
| Other | Forward as NDJSON command via UDS, read one response |

Tool count control: `--skills` and `--connectors` flags to specify exposed scope.

### 4.9 Plugin Runtime Shim

**Problem:** OpenClaw plugins (WeChat, Feishu) are not standalone CLIs — they `require("openclaw/plugin-sdk")` and cannot run outside OpenClaw.

**Solution:** A fake `plugin-sdk` module that impersonates the OpenClaw runtime:

```
Go Daemon (process management + UDS)
    │
    ├─ stdin  → write commands to shim (e.g., send reply)
    ├─ stdout ← read events from shim (e.g., message received)
    │
    └─ Subprocess: tsx c2c-shim.js <plugin-name>    ← tsx for ESM + TypeScript
                 │
                 ├─ NODE_PATH → shim/node_modules (fake SDK) + global node_modules
                 │
                 └─ import("@tencent-weixin/openclaw-weixin")   ← dynamic import
                      │
                      └─ require("openclaw/plugin-sdk") → our shim
                           │
                           └─ Plugin thinks it's running inside OpenClaw
```

**Module path hijacking:** `NODE_PATH` injects `shim/node_modules/` containing:
- `@openclaw/plugin-sdk/index.js` — main shim implementation
- `openclaw/plugin-sdk/index.js` — re-exports `@openclaw/plugin-sdk`

**Runtime function classification:**

| Category | Function | Shim behavior |
|----------|----------|---------------|
| Core | `reply.dispatchReplyFromConfig` | Forward to Go via NDJSON, wait for stdin reply (5min timeout) |
| Core | `reply.createReplyDispatcherWithTyping` | Wrap dispatch + typing indicator |
| Core | `media.saveMediaBuffer` | Save to `C2C_STORAGE_DIR/media/` |
| Minimal | `routing.resolveAgentRoute` | Return default route |
| Minimal | `session.recordInboundSession` | Write local JSON |
| Stub | `commands.*`, `reply.resolveHumanDelayConfig` | Return defaults |

**Verified compatible plugins:**
- `@tencent-weixin/openclaw-weixin` (WeChat)
- `@larksuite/openclaw-lark` (Feishu) — requires 20+ utility function stubs

### 4.10 Capability Discovery

**Problem:** Plugins expose rich capabilities (send text, send media, multi-account, agent prompts), but c2c currently treats all connectors as opaque message pipes. Consumers (CLI, MCP agents, scripts) have no way to discover what a plugin can do or how to invoke its features.

**Example — WeChat plugin capabilities (hidden today):**

| Capability | Plugin source | Currently exposed? |
|------------|--------------|-------------------|
| Receive messages | `gateway.startAccount` → long-poll | ✅ via `message.received` event |
| Send text reply | `outbound.sendText` | ❌ Only via `get_reply` response |
| Send media (image/file) | `outbound.sendMedia` (local path + remote URL) | ❌ Not exposed |
| Multi-account | `config.listAccountIds` / `resolveAccount` | ❌ Not exposed |
| Agent prompt hints | `agentPrompt.messageToolHints` | ❌ Not exposed to MCP |

#### Design: Three-layer capability surfacing

```
Layer 1: Plugin declares capabilities (already exists in plugin source)
    │
    ▼
Layer 2: Shim translates to MCP Tool Schema (new — shim does ALL translation)
    │
    ▼
Layer 3: Daemon caches & passes through (new — zero plugin-specific logic in Go)
    │
    ▼
Consumers: MCP ListTools / CLI `c2c call` / UDS query
```

#### Constraint 1: Schema Translation in Shim Layer

> Consensus (2026-03-23, human + Gemini + Claude): All plugin-specific translation happens in the Node.js shim. Go daemon is schema-agnostic.

OpenClaw plugins have proprietary formats (`agentPrompt.messageToolHints`, `ChannelPlugin` interface, `capabilities` object). None of these leak to Go.

The shim translates plugin capabilities into **standard MCP Tool Schema (JSON Schema)** and emits a new `discovery` message type:

```jsonl
{"type":"discovery","source":"wechat","payload":{
  "tools": [
    {
      "name": "wechat_send_text",
      "description": "Send a text message to a WeChat user. The recipient must be a valid WeChat ID ending in @im.wechat.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "to": {"type": "string", "description": "Recipient WeChat ID (e.g. wxid_xxx@im.wechat)"},
          "text": {"type": "string", "description": "Message content (max 4000 chars)"}
        },
        "required": ["to", "text"]
      }
    },
    {
      "name": "wechat_send_media",
      "description": "Send an image or file to a WeChat user. Accepts a local absolute path or an HTTPS URL. CDN upload is handled automatically.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "to": {"type": "string", "description": "Recipient WeChat ID"},
          "media": {"type": "string", "description": "Absolute local path (/tmp/photo.png) or HTTPS URL"},
          "text": {"type": "string", "description": "Optional caption text"}
        },
        "required": ["to", "media"]
      }
    }
  ]
}}
```

Go daemon receives `discovery` → caches tools in memory → serves them via MCP `ListTools`. **Daemon contains zero plugin-specific logic.**

#### Constraint 2: Dynamic Registration & Deregistration

Capabilities are **stateful** — they follow the connector lifecycle:

```
c2c connect wechat
    → shim starts → plugin.register() → shim emits "discovery"
    → daemon caches tools → MCP ListTools includes wechat_send_text, wechat_send_media

c2c stop wechat  (or plugin crash)
    → daemon evicts all tools with source="wechat"
    → MCP ListTools no longer includes wechat tools
```

This ensures MCP clients (Claude Code, Gemini CLI) always see an accurate, real-time view of available tools. No stale tools from crashed connectors.

**Implementation in daemon:**
```go
// In-memory tool registry (Go side)
var toolRegistry sync.Map  // source -> []MCPTool

// On "discovery" message from shim stdout:
toolRegistry.Store(msg.Source, msg.Payload.Tools)

// On connector stop/crash:
toolRegistry.Delete(name)

// On MCP ListTools request:
var allTools []MCPTool
toolRegistry.Range(func(key, value interface{}) bool {
    allTools = append(allTools, value.([]MCPTool)...)
    return true
})
```

#### Constraint 3: Generic CLI Invocation (`c2c call`)

> Consensus (2026-03-23): No hardcoded business commands like `c2c send`. The CLI is a generic RPC client.

**Format:**
```bash
c2c call <connector> <tool-name> [json-args]
```

**Examples:**
```bash
# Send a text message
c2c call wechat wechat_send_text '{"to":"wxid_123@im.wechat","text":"hello"}'

# Send an image
c2c call wechat wechat_send_media '{"to":"wxid_123@im.wechat","media":"/tmp/photo.png"}'

# List available tools for a connector
c2c call wechat --list-tools
```

**Data flow:**
```
CLI → UDS (command: invoke_tool, tool: wechat_send_text, args: {...})
  → Go Daemon → stdin NDJSON → Node.js Shim
  → shim bridge function → plugin.outbound.sendText()
  → result → stdout NDJSON → Daemon → UDS → CLI
```

The CLI never interprets tool semantics. It's a thin RPC shell over UDS, the same protocol MCP and `c2c echo` use.

#### Constraint 4: Surface Only What Agents Need

> Consensus (2026-03-23): Shim filters internal details. Agents get the simplest possible interface.

**Exposed to agents (via MCP tools):**
- `wechat_send_text` — send a text message
- `wechat_send_media` — send an image/file (CDN upload handled internally)

**Hidden from agents (encapsulated in shim bridge):**
- `contextToken` management (automatically resolved per conversation)
- CDN upload flow (`downloadRemoteImageToTemp` → `sendWeixinMediaFile`)
- Multi-account routing (shim picks the active account)
- Slash commands (`/echo`, `/toggle-debug` — internal debugging)
- Session guard (`assertSessionActive` — handled transparently)
- Text chunk splitting (shim splits at `textChunkLimit: 4000`)

The shim's bridge functions wrap plugin internals into clean, agent-friendly operations. Agent sees: "send text to user X". Shim handles: resolve account, get context token, assert session, call API, handle CDN if media.

#### Architecture Principles (Summary)

| # | Principle | Enforcement |
|---|-----------|-------------|
| 1 | **Schema translation in shim** | Shim emits MCP Tool Schema JSON. Daemon is schema-agnostic — zero plugin-specific Go code. |
| 2 | **Dynamic lifecycle** | Tools registered on connect, evicted on stop/crash. MCP always reflects live state. |
| 3 | **Generic RPC CLI** | `c2c call` is a universal tool invoker. No hardcoded business commands. |
| 4 | **Agent-facing simplicity** | Shim filters internals. Agents get minimal, high-level tools. Complexity lives in bridge functions. |
| 5 | **Plugin-agnostic** | New plugins (Feishu, Discord) auto-surface tools. Zero daemon changes per plugin. |
| 6 | **Graceful degradation** | Plugins without discovery support fall back to opaque pipe behavior. |

## 5. Target Platforms

- macOS (darwin/arm64, darwin/amd64)
- Linux (linux/amd64, linux/arm64)
- **Windows not supported**

## 6. Roadmap

### Phase 1: Core Framework + WeChat MVP ✅ Complete

### Phase 1.5: Capability Discovery (next)
- Shim introspects `ChannelPlugin` after registration → translates to MCP Tool Schema → emits `discovery` message
- New NDJSON message type: `discovery` (tools array with JSON Schema `inputSchema`)
- Daemon caches tool schemas per connector (schema-agnostic, zero plugin-specific code)
- Dynamic tool lifecycle: register on connect, evict on stop/crash
- MCP `ListTools` returns live tools from all active connectors
- MCP `CallTool` → forward as `invoke_tool` command via UDS → shim bridge → plugin method
- New CLI command: `c2c call <connector> <tool> [json-args]` (generic RPC client)
- Shim bridge functions encapsulate plugin internals (context tokens, CDN, session guards)
- `c2c info <plugin>` shows discovered tool schemas

### Phase 2: Plugin Expansion + Security Hardening
- More connectors (Feishu, Discord)
- More skills (search)
- Runtime checksum verification
- Runtime sandbox (sandbox-exec, seccomp-bpf)
- macOS Keychain integration
- Plugin update management (`c2c update`)

### Phase 3: Stabilization + Ecosystem
- Daemon management improvements
- TUI dashboard (bubbletea)
- Plugin contribution guide
- Homebrew / APT distribution

## 7. Design Decisions

| Decision | Conclusion | Rationale |
|----------|-----------|-----------|
| Compatibility depth | Thin (os/exec + npx) | Fast ecosystem capture; rewriting is "heavy tax" |
| Daemon mode | Self-managed processes (PID files) | Simplest cross-platform, no external deps |
| MCP priority | Day 1 core feature | Primary consumers are Claude Code / Gemini CLI |
| IPC | Pipe for control, UDS for data | Full-duplex + multi-consumer + reconnectable |
| UDS protocol | NDJSON | Native Go Scanner + JS readline support |
| Node runner | tsx first, node fallback | Plugins ship ESM + TS source, no pre-compiled JS |
| Connect default | Foreground, `-b` for background | First login needs QR code visibility |
| Shutdown timeout | SIGTERM → 3s → SIGKILL | Prevent blocking from pending `get_reply` |
| CLI vs runtime pkg | Auto-detect and install both | `-cli` suffix = installer wrapper, strip for runtime |
| Security strategy | Phase 1: hash + declarative perms; Phase 2: runtime sandbox | Layered defense, don't block delivery |
| Capability discovery | Shim introspects at runtime, not from static config | Captures actual plugin state; adding new plugins needs zero daemon changes |
| Schema translation | All in shim (Node.js), daemon is schema-agnostic | Go layer has zero plugin-specific logic; shim outputs standard MCP Tool Schema |
| Tool lifecycle | Dynamic register/deregister following connector lifecycle | MCP clients always see accurate live state; no stale tools |
| CLI tool invocation | Generic `c2c call` RPC, no hardcoded business commands | CLI is a universal RPC client; same protocol as MCP and UDS |
| Agent surface area | Minimal high-level tools; internals hidden in shim bridge | Agents get send_text/send_media; CDN, tokens, sessions encapsulated |

## 8. Open Questions

1. **Which search skill:** Tavily vs Brave Search vs other?
2. **Browser automation timing:** Phase 2 or later? (Chromium dependency conflicts with "lightweight" positioning)
3. **File-watcher plugins:** Long-running but not long-connection — need a third plugin model? (Phase 2)
