# Claw2Cli 设计文档

> 最后更新：2026-03-23（Phase 1 实测微信插件后同步）

## 目录

- [1. 项目定位](#1-项目定位)
- [2. 上游生态：OpenClaw](#2-上游生态openclaw)
- [3. 两类插件模型](#3-两类插件模型)
- [4. 技术架构](#4-技术架构)
  - [4.1 语言与技术栈](#41-语言与技术栈)
  - [4.2 兼容层策略](#42-兼容层策略薄兼容-thin-compatibility)
  - [4.3 JS-Go 桥接通道](#43-js-go-桥接通道)
  - [4.4 守护进程模型](#44-连接型插件的守护进程模型)
  - [4.5 整体架构](#45-整体架构)
  - [4.6 安全模型](#46-安全模型分层防御)
  - [4.7 凭证与存储隔离](#47-凭证与存储隔离)
  - [4.8 MCP Server](#48-mcp-serverday-1-特性)
  - [4.9 Plugin Runtime Shim](#49-plugin-runtime-shim俄罗斯套娃架构)
- [5. 目标平台](#5-目标平台)
- [6. MVP 范围](#6-mvp-范围)
- [7. 开发路线图](#7-开发路线图)
- [8. 已达成共识的决策](#8-已达成共识的决策)
- [9. 待定决策](#9-待定决策)
- [10. 协作模式](#10-协作模式)

## 1. 项目定位

Claw2Cli 是一个 **Go 语言编写的 CLI 兼容层**，从 OpenClaw 生态中提取大厂提供的高质量插件，将其"翻译"为标准 CLI 工具。

**核心洞察：** 生产级开发者不会使用 OpenClaw（太重、太不安全），但腾讯、字节、Kimi 等大厂正在为 OpenClaw 生态发布有价值的插件。Claw2Cli 捕获这些生态价值，以 CLI 形式提供给任何能调用 shell 的消费者 — Claude Code、Python 脚本、CI/CD 流水线、手动操作等。

**不是什么：**
- 不是 OpenClaw 的 fork 或精简版
- 不是要内嵌 Node.js 运行时
- 不是要重写插件逻辑

## 2. 上游生态：OpenClaw

| 属性 | 值 |
|------|-----|
| 仓库 | github.com/openclaw/openclaw |
| 规模 | 330k stars, 78 extensions, 52 skills |
| 技术栈 | TypeScript / Node.js / pnpm |
| 插件协议 | SKILL.md（YAML frontmatter + Markdown body） |
| 安全模型 | Docker 沙箱（group/channel sessions） |
| 许可证 | MIT |

### SKILL.md 协议

```yaml
---
name: skill-name
description: 一句话描述
emoji: 🔧
# 可选：binary requirements, install methods
---

# 使用说明（Markdown，建议 < 500 行）
```

可选附加资源：
- `scripts/` — 确定性操作的可执行代码
- `references/` — 按需加载的参考文档
- `assets/` — 模板和输出文件

加载策略：metadata（~100 词）始终加载 → SKILL.md body 按需加载（< 5k 词） → 附加资源按需加载。

## 3. 两类插件模型

### 3.1 技能型插件 (Skill)

- **特征：** 无状态，调用即走
- **例子：** 搜索、翻译、代码分析、天气查询
- **调用方式：** `c2c run <skill-name> [--args]`
- **生命周期：** 启动子进程 → 传入参数 → 收集输出 → 进程退出
- **权限：** 受限 — 仅允许网络访问 + 临时文件写入，禁止访问宿主文件系统

### 3.2 连接型插件 (Connector)

- **特征：** 有状态，长连接
- **例子：** 微信、飞书、Discord、Slack
- **调用方式：** `c2c connect <connector-name>` → 前台运行（默认），可见 QR 码等交互提示；加 `-b` 后台运行
- **生命周期：** 守护进程维持长连接心跳 → CLI 通过 IPC 交互 → Ctrl+C 或 `c2c stop` 退出
- **权限：** 受控高权限 — 允许网络访问 + 持久状态存储 + 凭证管理

### 权限差异总结

| 维度 | Skill | Connector |
|------|-------|-----------|
| 网络访问 | ✅ 受限 | ✅ 完全 |
| 文件系统 | 临时目录 | 专属数据目录 |
| 持久状态 | ❌ | ✅ session 持久化 |
| 凭证存储 | ❌ | ✅ 加密存储 |
| 进程模型 | 一次性子进程 | 守护进程 |

## 4. 技术架构

### 4.1 语言与技术栈

**语言：** Go 1.22+（实际开发使用 Go 1.26.1）
**二进制大小：** ~8MB（单二进制，无外部依赖）

理由：
- **单二进制分发** — 无需用户安装 Node.js 运行时（插件自身的 Node 依赖通过 npx 按需获取）
- **goroutine** — 天然适合连接型插件的多长连接并发管理
- **跨平台编译** — macOS (darwin/arm64, darwin/amd64) + Linux (linux/amd64, linux/arm64)
- **CLI 生态成熟** — cobra, viper 等

**核心依赖：**
| 库 | 用途 |
|----|------|
| `github.com/spf13/cobra` | CLI 框架 |
| `github.com/spf13/viper` | 配置管理（YAML） |
| `gopkg.in/yaml.v3` | YAML 解析 |
| `github.com/mark3labs/mcp-go` | MCP Server SDK |

### 4.2 兼容层策略：薄兼容 (Thin Compatibility)

> 三方共识（2026-03-22）：薄兼容是唯一正确路径。重写插件是"重税"，兼容层是"杠杆"。

Go 通过 `os/exec` 调用 npx 子进程，原生运行 OpenClaw 插件：

```
c2c run google-search --query "AI news"
    │
    ▼
Go 解析 SKILL.md → 构造 npx 调用命令 → 子进程执行 → 收集 stdout/stderr → 格式化输出
```

优势：
- **隔离性：** 插件崩溃仅影响子进程，不会带走 Go 主程序
- **版本控制：** 从 SKILL.md 解析版本需求，动态调用 `npx @package@version`
- **零 JS 依赖：** Go 二进制本身不需要 Node.js，插件的 Node 依赖通过 npx 按需获取

**ESM + TypeScript 支持：** OpenClaw 大厂插件（微信、飞书）使用 `"type": "module"` + TypeScript 源码发布（无预编译 JS）。Claw2Cli 通过 [tsx](https://github.com/privatenumber/tsx) 替代 node 运行 shim，支持 ESM `import()` 和 TypeScript 直接加载。`resolveNodeRunner()` 优先使用全局 tsx，若未安装则自动执行 `npm install -g tsx`，最后兜底到 node（TypeScript 插件可能无法加载）。

**模块加载策略：** shim 使用 `await import()` 动态导入插件（支持 ESM），若失败则回退到 `require()`（支持 CommonJS 插件），确保两种模块格式都能兼容。

**CLI 包 vs 运行时包：** 部分插件拆分为两个 npm 包——CLI 安装器（如 `@tencent-weixin/openclaw-weixin-cli`）和实际运行时（如 `@tencent-weixin/openclaw-weixin`）。`c2c install` 和 daemon 均通过 `resolvePluginPackage()` 自动识别并安装两者。

### 4.3 JS-Go 桥接通道

两类插件使用不同的通信方式：

**技能型 — stdin/stdout pipe（简单直接）：**
```
Go → os/exec → npx skill-plugin --json → stdout → Go 解析 JSON 结果
```

**连接型 — stdin/stdout + Unix Domain Socket（全双工）：**
```
Go daemon
    │
    ├─ stdin/stdout pipe ← 控制信令（启动、停止、状态查询）
    │
    └─ Unix Domain Socket ← 数据流（消息收发，全双工）
         路径：~/.c2c/sockets/<connector-name>.sock
```

为什么连接型需要 UDS 而不是纯 pipe：
- pipe 是半双工，UDS 是全双工
- UDS 支持多消费者同时连接（CLI 查看 + MCP 转发可并行）
- UDS 有文件系统路径，支持 `c2c attach wechat` 重新连接到已有 daemon

**UDS 上的通信协议：NDJSON (Newline Delimited JSON)**

> 三方共识（2026-03-22）：NDJSON 最符合 Go Scanner 和 JS readline 的处理习惯。

每条消息一行 JSON，末尾换行。所有消息必须包含 `source` 字段标识来源插件（支持多 connector 并发分发）：

```jsonl
{"type":"event","source":"wechat","topic":"message.received","payload":{...},"ts":1711100000}
{"type":"command","source":"wechat","action":"send_message","payload":{...},"id":"req-001"}
{"type":"response","source":"wechat","payload":{...},"id":"req-001"}
{"type":"error","source":"feishu","code":"AUTH_FAILED","message":"...","ts":1711100000}
{"type":"log","source":"wechat","level":"info","message":"heartbeat ok","ts":1711100000}
```

- `source` — 插件标识符，多 connector 并发时用于消息分发和日志过滤
- `command` + `response` 通过 `id` 关联，实现请求-响应模式
- `event` 是服务端推送（如收到微信消息）
- `log` 用于调试，可按 level 和 source 过滤
- `error` 用于不可恢复的错误通知

**消息缓冲：** MVP 阶段 daemon 使用 `sync.Map` 管理客户端连接，收到消息后直接广播给所有连接的客户端。后续可按需引入缓冲队列。

**环境变量注入：** 所有插件子进程继承宿主环境，并额外注入：
| 变量 | 说明 |
|------|------|
| `C2C_PLUGIN_NAME` | 插件名称 |
| `C2C_PLUGIN_TYPE` | `skill` 或 `connector` |
| `C2C_STORAGE_DIR` | 插件专属存储路径（`~/.c2c/storage/<name>`） |
| `C2C_BASE_DIR` | c2c 根目录（`~/.c2c`） |
| `C2C_PLUGIN_SOURCE` | 原始 npm 包名（如 `@tencent-weixin/openclaw-weixin-cli@latest`） |
| `NODE_PATH` | `shim/node_modules` + 全局 `node_modules`（模块路径劫持） |

### 4.4 连接型插件的守护进程模型

> 三方共识：自管理进程（Process Tracking），不依赖 systemd/launchd。

**实现方式：** 由于 Go 不支持真正的 fork，采用 **隐藏子命令重新调用自身** 的模式：

```
c2c connect wechat          # 前台模式（默认）：直接运行 daemon，终端可见 QR 码
c2c connect wechat -b       # 后台模式：通过 Setsid 脱离终端

前台模式：
    Go 直接调用 runDaemon()
    │
    ├─ 子进程: tsx c2c-shim.js wechat（支持 ESM + TypeScript）
    ├─ stdout → 解析 NDJSON，打印日志/事件到终端
    ├─ stderr → 直通终端（QR 码等交互输出）
    ├─ Ctrl+C → SIGTERM → 等 3s → SIGKILL
    └─ 监听 UDS（支持 attach 并行连接）

后台模式：
    Go 执行 `c2c _daemon wechat`（隐藏子命令），通过 Setsid 脱离终端
    │
    ├─ 子进程: tsx c2c-shim.js wechat
    ├─ stdout/stderr → 广播为 NDJSON 消息
    ├─ 监听 UDS (~/.c2c/sockets/wechat.sock)
    ├─ PID 记录到 ~/.c2c/pids/wechat.pid
    ├─ 元数据记录到 ~/.c2c/pids/wechat.json
    └─ 日志写入 ~/.c2c/logs/wechat.log

c2c list          → 扫描 PID 文件，检查进程存活
c2c attach wechat → 连接到 UDS（优先通过 PID 查 socket 路径，无 PID 文件时直连 socket，兼容前台模式）
c2c status        → 表格显示活跃连接器（名称、PID、运行时间）
c2c logs wechat -f → tail 日志文件
c2c stop wechat   → SIGTERM → 等待 5s → SIGKILL，清理 PID/socket/metadata 文件
```

### 4.5 整体架构

```
用户 / Agent / 脚本
        │
        ▼
┌──────────────────┐
│     c2c CLI       │  ← Go 单二进制
├──────────────────┤
│  Command Router   │  ← run / connect / list / attach / stop / mcp
├──────┬───────────┤
│ Skill │ Connector │
│Runner │ Manager   │
│(pipe) │ (daemon)  │
├──────┴───────────┤
│   Plugin Shim     │  ← SKILL.md 解析 + npx/node 桥接
├──────────────────┤
│ Permission Guard  │  ← Skill: 受限 / Connector: 受控高权限
├──────────────────┤
│  MCP Server       │  ← JSON-RPC over stdio，暴露为 MCP tools
└──────────────────┘
        │
        ▼
  OpenClaw 插件（npm 包，原生运行）
```

### 4.6 安全模型：分层防御

> 三方共识：安全与薄兼容不冲突，关键是分层实现、分阶段交付。

**MVP（Phase 1）— 静态防御：**
- **包完整性校验：** `c2c install` 时记录 npm 包 sha512 hash（已实现）。运行前校验尚未实现，列入 Phase 2
- **声明式权限清单：** c2c 在自己的 manifest 层面维护权限声明（不侵入上游 SKILL.md 协议），运行前静态拦截（已实现，Permission Guard 在 `os/exec` 之前检查）
- **安装预检：** `c2c install` 在安装前验证 node/npm 可用性和 shim 文件完整性，提前暴露环境问题

权限清单通过 c2c 自有的 manifest 文件管理（不修改上游 SKILL.md）：
```yaml
# ~/.c2c/plugins/wechat/manifest.yaml
source: "@tencent-weixin/openclaw-weixin-cli@latest"
type: connector
permissions:
  - network
  - fs:~/.c2c/storage/wechat   # 限定路径
  - credential:keychain
checksum: "sha512:abc123..."
```
插件首次安装时自动生成默认 manifest，用户可手动收紧权限。Skill Runner 在 `os/exec` 之前检查权限清单，不符合则直接拒绝执行。

**Phase 2 — 运行时沙箱：**
- macOS: `sandbox-exec`（Apple 原生沙箱，用户态，无需 root）
- Linux: `seccomp-bpf`（轻量 syscall 过滤，无需 root）
- 按插件类型应用不同 profile（Skill 严格限制，Connector 受控放行）

**明确不做的：**
- ptrace 监控 — 性能损耗大，跨平台差异大，维护成本极高
- eBPF — 需要 root，macOS 不支持
- 内嵌 JS 引擎 — 与薄兼容策略矛盾

### 4.7 凭证与存储隔离

连接型插件（微信、飞书）需要持久化 session token。OpenClaw 的插件通常将凭证散落在各自目录，安全性差。

**MVP — 目录隔离 + 文件权限：**
```
~/.c2c/
  ├── plugins/<name>/manifest.yaml   # 插件元数据 + 权限清单
  ├── storage/<name>/                # 插件专属数据，权限 0700
  ├── sockets/<name>.sock            # UDS（仅 connector）
  ├── pids/<name>.pid                # PID 文件（仅 connector）
  ├── pids/<name>.json               # 元数据（仅 connector）
  ├── logs/<name>.log                # daemon 日志（仅 connector）
  └── config.yaml                    # 全局配置
```

- 每个插件的子进程通过环境变量 `C2C_STORAGE_DIR` 获知专属存储路径
- Permission Guard 确保插件只能访问自己的目录（manifest 中声明 `fs:~/.c2c/storage/<name>`）
- 目录权限强制设为 0700（仅 owner 可读写）

**Phase 2 — macOS Keychain 集成：**
- 对于敏感字段（access_token 等），提供可选的 Keychain/keyring 加密存储
- 通过 `go-keyring` 库封装，对插件透明（c2c 代理存取）
- MVP 不做的原因：大厂插件自己管理 token 格式，c2c 无法替它们决定哪些字段敏感；Keychain 授权弹窗等边缘情况多

### 4.8 MCP Server（Day 1 特性）

作为标准 MCP Server 运行时，Claw2Cli 将已安装插件暴露为 MCP tools，使 Claude Code 和 Gemini CLI 可直接调用。

配置示例（Claude Code `mcp_servers` 设置）：
```json
{
  "c2c": {
    "command": "c2c",
    "args": ["mcp", "serve"]
  }
}
```

**Skill 工具调用方式：** Agent 传入 `args` 参数（空格分隔的参数字符串），c2c 拆分后传给 npx 子进程。

**Connector 工具调用方式：** Agent 传入 `action` 参数，c2c 根据 action 值分发：
| action | 行为 |
|--------|------|
| `start` | 调用 `StartConnector` 启动 daemon |
| `stop` | 调用 `StopConnector` 关闭 daemon |
| `status` | 调用 `GetConnectorStatus` 返回 JSON 状态 |
| 其他 | 通过 UDS 转发为 NDJSON command，读取一条 response 返回 |

工具数量控制策略：
- 不一次性暴露所有插件
- 支持 `--skills` 和 `--connectors` 参数指定暴露范围
- 支持配置文件定义默认暴露集

### 4.9 Plugin Runtime Shim（"俄罗斯套娃"架构）

> 三方共识（2026-03-22）：方案 A（Plugin Runtime Shim）是唯一正确路径。
> 不 fork 插件代码（方案 C），不安装完整 OpenClaw（方案 B）。

**问题：** OpenClaw 大厂插件（微信、飞书）不是独立 CLI，而是 OpenClaw 的扩展模块。它们通过 `require("openclaw/plugin-sdk")` 获取运行时，无法脱离 OpenClaw 独立运行。

**解决方案：** 创建一个假的 `plugin-sdk` 模块，伪造 `PluginRuntime` 对象，让插件以为自己运行在 OpenClaw 里。

```
Go Daemon (进程管理 + UDS 对外)
    │
    ├─ stdin  → 写入命令给 shim（如：发送消息回复）
    ├─ stdout ← 读取事件从 shim（如：收到微信消息）
    │
    └─ 子进程: tsx c2c-shim.js <plugin-name>    ← tsx 支持 ESM + TypeScript
                 │
                 ├─ NODE_PATH 指向 shim/node_modules（假 SDK）+ 全局 node_modules
                 │
                 └─ import("@tencent-weixin/openclaw-weixin")   ← 动态 import
                      │
                      └─ require("openclaw/plugin-sdk") → 我们的 shim
                           │
                           └─ 以为自己在 OpenClaw 里运行
```

**模块路径劫持：** 通过 `NODE_PATH` 环境变量注入 `shim/node_modules/`，其中包含：
- `@openclaw/plugin-sdk/index.js` — 主 shim 实现
- `openclaw/plugin-sdk/index.js` — 转发到 `@openclaw/plugin-sdk`
- `openclaw/plugin-sdk/core.js` — 子路径导出

**Runtime 函数分类：**

| 类别 | 函数 | Shim 行为 |
|------|------|-----------|
| 核心 | `reply.dispatchReplyFromConfig` | 不调 LLM，通过 stdout NDJSON 转发给 Go，等 stdin 回复（5 分钟超时） |
| 核心 | `reply.createReplyDispatcherWithTyping` | 包装 dispatch + typing indicator |
| 核心 | `media.saveMediaBuffer` | 保存到 `C2C_STORAGE_DIR/media/` |
| 最小 | `routing.resolveAgentRoute` | 返回默认 route |
| 最小 | `session.recordInboundSession` | 写本地 JSON |
| 最小 | `reply.finalizeInboundContext` | 直接透传 |
| Stub | `commands.*`, `reply.resolveHumanDelayConfig` | 返回默认值 |

**可靠性处理：**
- **EPIPE 防护：** 当 Go daemon 关闭时 stdout pipe 断开，shim 的 `sendMessage()` 捕获 `EPIPE`/`ERR_STREAM_DESTROYED` 错误后静默停止写入，避免未处理异常导致进程崩溃
- **关闭超时：** daemon 发送 SIGTERM 后等待 3 秒，超时则 SIGKILL 强杀（防止 `get_reply` 等命令阻塞导致无法退出）

**已验证兼容的插件：**
- `@tencent-weixin/openclaw-weixin`（微信）— `require("openclaw/plugin-sdk")`
- `@larksuite/openclaw-lark`（飞书）— `require("openclaw/plugin-sdk")`，额外需要 20+ 个 utility 函数

## 5. 目标平台

- macOS (darwin/arm64, darwin/amd64)
- Linux (linux/amd64, linux/arm64)
- **不支持 Windows**

## 6. MVP 范围

**目标：** 微信插件跑通 CLI + MCP 两种模式

具体交付物：
1. `c2c connect wechat` — 启动微信连接，基于 `@tencent-weixin/openclaw-weixin-cli`
2. `c2c mcp serve` — 将微信能力暴露为 MCP tools
3. `c2c list` / `c2c info <plugin>` — 插件发现与信息查看
4. 基本的权限隔离框架

## 7. 开发路线图

### Phase 1：核心框架 + 微信 MVP ✅ 已完成
- [x] Go 项目脚手架（cobra CLI, 11 个命令 + 隐藏 _daemon）
- [x] SKILL.md 解析器 + manifest.yaml 解析器
- [x] Plugin Shim（npx 子进程桥接，带超时和权限检查）
- [x] Connector 守护进程管理（start/stop/attach/status/logs）
- [x] MCP Server（mcp-go，动态工具注册，支持 --skills/--connectors 过滤）
- [x] 插件安装（`c2c install`，自动生成 manifest，记录 checksum，预检 node/npm/shim，预装运行时包）
- [x] NDJSON 协议层（5 种消息类型 + source 字段 + 编解码器）
- [x] 声明式权限守卫（运行前拦截）
- [x] 存储目录隔离（0700 权限）
- [x] ESM + TypeScript 支持（tsx 运行 shim，动态 import，自动安装 tsx）
- [x] 前台/后台双模式连接（默认前台可见 QR 码，`-b` 后台守护进程）
- [x] 优雅关闭（Ctrl+C → SIGTERM → 3s 超时 → SIGKILL）
- [x] 微信插件实测通过（安装、扫码登录、Gateway 启动）
- [x] 单元测试覆盖率：internal/ 包 63.1%（executor 90.8%, mcp 85.9%, parser/config 100%）

### Phase 2：插件扩展 + 安全加固
- 更多 Connector 适配（飞书、Discord）
- 更多 Skill 适配（搜索类）
- 运行前 checksum 校验（install 时已记录，运行前验证待实现）
- 运行时沙箱（macOS sandbox-exec, Linux seccomp-bpf）
- macOS Keychain 凭证集成
- 插件更新管理（`c2c update <plugin>`）

### Phase 3：稳定化 + 生态
- Daemon 进程管理优化
- TUI 监控面板（bubbletea）
- 插件贡献指南
- Homebrew / APT 分发

## 8. 已达成共识的决策

| 决策点 | 结论 | 理由 |
|--------|------|------|
| 兼容层深度 | 薄兼容（os/exec + npx） | 快速占领生态，重写是"重税" |
| Daemon 模式 | 自管理进程（PID 文件 + 进程表） | 跨平台最简，无外部依赖 |
| MCP 优先级 | Day 1 核心特性 | 最大客户是 Claude Code / Gemini CLI |
| IPC 方式 | 控制信令用 pipe，数据流用 UDS | 全双工 + 多消费者 + 可重连 |
| MVP 插件 | 微信（Connector）+ 搜索类（Skill） | 微信证明长连接价值，搜索证明技能型价值 |
| UDS 协议 | NDJSON（行分隔 JSON） | Go Scanner + JS readline 天然支持 |
| 安全策略 | MVP 做 hash 校验 + 声明式权限；Phase 2 做运行时沙箱 | 分层防御，不阻塞交付 |
| 权限清单 | c2c 自有 manifest.yaml，不侵入上游 SKILL.md | 保持上游兼容性 |
| NDJSON source 字段 | 所有消息必须携带 `source` 标识来源插件 | 多 connector 并发分发 |
| 凭证存储 | MVP 用目录隔离 (0700)；Phase 2 可选 Keychain | 比 OpenClaw 散落存储安全，不过度工程 |
| TUI | Phase 3，不进 MVP | MVP 核心用户是 Agent 不是人类，先 `c2c status` + `c2c logs -f` |
| 配置格式 | YAML | manifest 是 YAML，SKILL.md frontmatter 是 YAML，统一格式 |
| Daemon 实现 | 隐藏子命令 `c2c _daemon` + Setsid | Go 不支持 fork，重新调用自身最可靠 |
| Node 运行器 | tsx 优先，node 兜底 | 大厂插件发布 ESM + TS 源码，无预编译 JS，tsx 是最轻量的加载方案 |
| connect 默认模式 | 前台运行，`-b` 切后台 | 首次登录需看 QR 码，前台更直觉；已登录后可 `-b` 后台 |
| 关闭超时 | SIGTERM → 3s → SIGKILL | 插件可能阻塞在 get_reply 等待，不能无限等 |
| CLI 包 vs 运行时包 | 自动识别并安装两者 | 如 `-cli` 是安装器，实际运行时是去掉 `-cli` 的包 |

## 9. 待定决策

> 以下问题待后续讨论确定

1. **搜索类 Skill 具体选哪个：** Tavily vs Brave Search vs 其他？
2. **浏览器自动化何时引入：** Phase 2 还是更晚？（依赖 Chromium，与"轻量"定位有张力）
3. **File-Watcher 类插件分类：** 长时间运行但非长连接，是否需要第三类插件模型？（Phase 2 讨论）

## 10. 协作模式

本项目由 **人类开发者**、**Gemini** 与 **Claude Code** 三方协作开发。
