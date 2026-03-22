# Claw2Cli (c2c)

[中文文档](README_CN.md)

> Wrap OpenClaw plugins as standard CLI tools. No Docker, no browser, just a binary.

## What is this

[OpenClaw](https://github.com/openclaw/openclaw) is a popular AI assistant platform with 78 extensions and 52 skills. Major companies like Tencent, ByteDance, and Kimi are building high-quality plugins for it — WeChat integration, Feishu bots, web search, and more.

The problem: OpenClaw itself is heavy, complex, and has security concerns. Production developers won't run it.

**Claw2Cli** solves this by extracting these plugins and exposing them as plain CLI commands. Your Go binary calls `npx` under the hood, so plugins run natively without modification. Any tool that can call a shell command — Claude Code, Gemini CLI, Python scripts, CI pipelines — gets instant access to these capabilities.

## Quick Start

```bash
# Install
go install github.com/YangZhengCQ/Claw2cli@latest

# Install a plugin
c2c install @tencent-weixin/openclaw-weixin-cli --type connector

# Start WeChat connector (foreground — shows QR code for login)
c2c connect wechat

# Discover what the plugin can do
c2c call wechat --list-tools

# Invoke a tool
c2c call wechat wechat_send_text '{"to":"user@im.wechat","text":"hello"}'

# Or run in background
c2c connect wechat -b

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
| `c2c connect <connector>` | Start a connector (foreground by default, `-b` for background) |
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

```
User / Agent / Script
        |
        v
+------------------+
|    c2c CLI       |  <- Go single binary
+------------------+
| Command Router   |  <- run / connect / list / echo / mcp
+------+-----------+
| Skill | Connector |
|Runner | Manager   |
|(pipe) | (daemon)  |
+------+-----------+
| Plugin Shim      |  <- SKILL.md parser + tsx/npx bridge
+------------------+
| Permission Guard |  <- manifest-based access control
+------------------+
| MCP Server       |  <- JSON-RPC over stdio
+------------------+
        |
        v
  OpenClaw plugins (npm packages, run natively)
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

In principle, any OpenClaw-compatible npm package can be installed. Plugins are auto-introspected at runtime — no hardcoded plugin logic in the Go binary.

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
