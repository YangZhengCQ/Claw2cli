# Claw2Cli (c2c)

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
# Pre-flight checks run automatically: verifies node/npm and shim files

# Start WeChat connector (foreground — shows QR code for login)
c2c connect wechat

# Or run in background
c2c connect wechat -b

# Check status
c2c status

# Or use a skill plugin
c2c install @some-scope/openclaw-search
c2c run search --query "AI news"
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
| `c2c status` | Show status of running connectors |
| `c2c logs <connector> -f` | Tail connector logs |
| `c2c list` | List installed plugins |
| `c2c info <plugin>` | Show plugin details |
| `c2c install <package>` | Install an OpenClaw plugin |
| `c2c mcp serve` | Start MCP server over stdio |

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
|    c2c CLI       |  <- Go binary
+------------------+
| Command Router   |  <- run / connect / list / mcp
+------+-----------+
| Skill | Connector |
|Runner | Manager   |
|(pipe) | (daemon)  |
+------+-----------+
| Plugin Shim      |  <- SKILL.md parser + npx bridge
+------------------+
| Permission Guard |  <- manifest-based access control
+------------------+
| MCP Server       |  <- JSON-RPC over stdio
+------------------+
        |
        v
  OpenClaw plugins (npm packages, run natively)
```

See [docs/DESIGN.md](docs/DESIGN.md) for the full design document.

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

- **Package integrity**: SHA-512 checksum recorded on install, verified before each run
- **Permission manifests**: Each plugin declares capabilities in `manifest.yaml`; unauthorized actions are blocked before execution
- **Storage isolation**: Each plugin gets its own data directory (`~/.c2c/storage/<name>/`) with 0700 permissions
- **Process isolation**: Plugins run as separate subprocesses via `os/exec`

Runtime sandboxing (seccomp-bpf on Linux, sandbox-exec on macOS) is planned for a future release.

## Supported Plugins

This is an early-stage project. Currently tested with:

| Plugin | Type | Source |
|--------|------|--------|
| WeChat | Connector | `@tencent-weixin/openclaw-weixin-cli` |

More plugins will be added as the project matures. In principle, any OpenClaw-compatible npm package can be installed.

## Development

```bash
git clone https://github.com/YangZhengCQ/Claw2cli.git
cd claw2cli
make build    # Build binary
make test     # Run tests
make lint     # Run go vet
make install  # Install to $GOPATH/bin
```

## License

MIT

## Credits

Built by a human developer, [Gemini](https://gemini.google.com), and [Claude Code](https://claude.ai/code) working together.
