# Claw2Cli (c2c)

[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![MCP Compatible](https://img.shields.io/badge/MCP-Ready-blueviolet)](https://modelcontextprotocol.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/YangZhengCQ/Claw2cli/actions/workflows/ci.yml/badge.svg)](https://github.com/YangZhengCQ/Claw2cli/actions)

[中文文档](README_CN.md)

> Bring your own Skills, we provide the Secure Runtime.
> Wrap OpenClaw plugins as standard CLI tools. No Docker, no browser, just your terminal.
>
> Last updated: 2026-03-28

## Why

Major companies (Tencent, ByteDance, Lark) are building high-quality AI plugins for [OpenClaw](https://github.com/openclaw/openclaw) — WeChat bots, Feishu integration, calendar management, web search, and more.

**The problem:** OpenClaw itself is heavy, insecure, and over-permissioned. Production developers won't run it. But the plugins are valuable.

**The solution:** Claw2Cli extracts these plugins and runs them in a thin compatibility layer. A Go daemon manages the process lifecycle, a Node.js shim impersonates the OpenClaw runtime, and the plugin thinks it's running inside OpenClaw — but its I/O, credentials, and permissions are strictly controlled through UDS pipes and isolated storage. No plugin code is modified. Zero hardcoded plugin logic in Go.

## Install

```bash
# Homebrew (macOS / Linux)
brew install YangZhengCQ/tap/c2c

# Or from source
go install github.com/YangZhengCQ/Claw2cli@latest

# Or download binary directly from GitHub Releases
# https://github.com/YangZhengCQ/Claw2cli/releases
```

## Quick Start

```bash

# Install a plugin
c2c install @tencent-weixin/openclaw-weixin-cli --type connector

# Start WeChat connector (runs as background daemon)
c2c connect wechat

# Discover what the plugin can do
c2c call wechat --list-tools

# Invoke a tool
c2c call wechat wechat_send_text '{"to":"user@im.wechat","text":"hello"}'

# Foreground mode (for QR login or debugging)
c2c connect wechat -f

# Check status
c2c status

# Skill plugins work too
c2c install @some-scope/openclaw-search
c2c run search --query "AI news"
```

**`--list-tools` output — capabilities are introspected at runtime, zero hardcoded plugin logic:**

```
$ c2c call wechat --list-tools

Discovered 2 tool(s) for "wechat":

  wechat_send_text
    Send a text message via wechat
      --to — Recipient ID
      --text — Message content (max 4000 chars)

  wechat_send_media
    Send an image or file. Accepts an absolute local path or an HTTPS URL.
      --to — Recipient ID
      --media — Absolute local path (/tmp/photo.png) or HTTPS URL
      --text — Optional caption text
```

## Prerequisites

- **Go 1.22+** (build only)
- **Node.js 18+** and npm (runtime — plugins are npm packages)
- **[tsx](https://github.com/privatenumber/tsx)** (auto-installed on first `connect` — needed for ESM + TypeScript plugins)
- **macOS** or **Linux** (Windows is not supported)

## CLI Reference

| Command | Description |
|---------|-------------|
| `c2c run <skill> [args]` | Run a skill plugin (one-shot) |
| `c2c connect <connector>` | Start a connector daemon (background by default, `-f` foreground for debugging) |
| `c2c stop <connector>` | Stop a running connector |
| `c2c attach <connector>` | Stream messages from a running connector |
| `c2c echo <connector>` | Test consumer that echoes back received messages |
| `c2c call <connector> <tool> [json]` | Invoke a discovered tool (generic RPC) |
| `c2c call <connector> --list-tools` | List tools discovered from a running connector |
| `c2c status` | Show status of running connectors |
| `c2c logs <connector> -f` | Tail connector logs |
| `c2c list` | List installed plugins |
| `c2c info <plugin>` | Show plugin details (+ discovered tools if running) |
| `c2c install <package>` | Install an OpenClaw plugin |
| `c2c update [plugin]` | Update installed plugins to latest version |
| `c2c mcp serve` | Start MCP server over stdio (includes discovered tools) |

## Capability Discovery

Plugins expose capabilities (send text, send media, etc.) that c2c **automatically discovers at runtime**. No hardcoded plugin logic in the Go binary.

```bash
# Start a connector
c2c connect wechat

# In another terminal — discover what it can do
c2c call wechat --list-tools
# Output:
#   wechat_send_text     Send a text message via wechat
#     --to       Recipient ID
#     --text     Message content (max 4000 chars)
#
#   wechat_send_media    Send an image or file...
#     --to       Recipient ID
#     --media    Absolute local path or HTTPS URL

# Invoke a tool
c2c call wechat wechat_send_text '{"to":"user@im.wechat","text":"hello"}'
```

The same tools are automatically exposed via MCP, so Claude Code and other agents can use them directly.

## Using with Claude Code (MCP)

Add to your Claude Code MCP settings:

```json
{
  "mcpServers": {
    "c2c": {
      "command": "c2c",
      "args": ["mcp", "serve"]
    }
  }
}
```

To expose only specific plugins:

```bash
c2c mcp serve --skills search,translate --connectors wechat
```

## Architecture

Three-layer isolation — the "Russian doll" model:

```
User / AI Agent (Claude Code, Gemini CLI, scripts)
      | (MCP protocol / CLI invocation)
      v
+-------------------------------------------+
|  Go Daemon (c2c core)                     |
|  Process management, UDS listener,        |
|  NDJSON routing, permission guard,        |
|  capability registry, MCP server          |
+-------------------------------------------+
      | (stdin/stdout pipe)
      v
+-------------------------------------------+
|  Node.js Shim (c2c-shim.js)              |
|  Fake @openclaw/plugin-sdk,              |
|  capability introspection -> MCP Schema,  |
|  tool invocation bridge                   |
+-------------------------------------------+
      | (plugin thinks it's in OpenClaw)
      v
+-------------------------------------------+
|  Original plugin (e.g., openclaw-weixin)  |
|  Unmodified npm package, runs natively    |
+-------------------------------------------+
```

## Plugin Model

**Skills** are stateless, one-shot tools (search, translate, code analysis). They run as a subprocess, return a result, and exit. Permissions are restricted to network access and temporary files.

**Connectors** are stateful, long-lived daemons (WeChat, Feishu, Discord). They maintain background connections, communicate via Unix Domain Socket (NDJSON protocol), and have access to persistent storage and credentials.

| | Skill | Connector |
|---|---|---|
| Lifecycle | Call and return | Background daemon |
| State | None | Session persistence |
| IPC | stdin/stdout pipe | Unix Domain Socket |
| Permissions | Restricted | Controlled elevated |

## Security

Claw2Cli takes a layered approach to security:

- **Package integrity**: SHA-512 checksum recorded on install
- **Permission manifests**: Each plugin declares capabilities in `manifest.yaml`; unauthorized actions are blocked before execution
- **Storage isolation**: Each plugin gets its own data directory (`~/.c2c/storage/<name>/`) with 0700 permissions
- **Process isolation**: Plugins run as separate subprocesses via `os/exec`

Runtime sandboxing (seccomp-bpf on Linux, sandbox-exec on macOS) is planned for a future release.

## Supported Plugins

| Plugin | Type | Source | Tools | Status |
|--------|------|--------|-------|--------|
| WeChat | Connector | `@tencent-weixin/openclaw-weixin-cli` | 2 | Full E2E (login, send/receive, MCP) |
| Feishu/Lark | Connector | `@larksuite/openclaw-lark` | 27 | Load + discovery verified |
| QQ Bot | Connector | `@tencent-connect/openclaw-qqbot` | 3 | Load + discovery verified |
| WeCom | Connector | `@wecom/wecom-openclaw-plugin` | 3 | Load + discovery + gateway verified |
| Web Search | Skill | `@ollama/openclaw-web-search` | 2 | Skill-only plugin, discovery verified |

6/7 tested plugins load successfully (39 tools total). In principle, any OpenClaw-compatible npm package can be installed — plugins are auto-introspected at runtime with zero hardcoded plugin logic in the Go binary.

## Development

```bash
git clone https://github.com/YangZhengCQ/Claw2cli.git
cd Claw2cli
make build    # Build binary
make test     # Run tests
make lint     # Run go vet
make install  # Install to $GOPATH/bin
```

## Documentation

| Document | Description |
|----------|-------------|
| [Design](docs/DESIGN.md) | Architecture, plugin model, design decisions |
| [Implementation](docs/IMPLEMENTATION.md) | Code structure, key functions, flows |
| [Pitfalls](docs/PITFALLS.md) | Lessons learned during development (Chinese) |

## License

MIT

## Credits

Built by a human developer, [Gemini](https://gemini.google.com), and [Claude Code](https://claude.ai/code) working together.
