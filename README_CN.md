# Claw2Cli (c2c)

[English](README.md)

> 把 OpenClaw 插件变成标准 CLI 工具。无 Docker，无浏览器，只需一个二进制。

## 这是什么

[OpenClaw](https://github.com/openclaw/openclaw) 是一个流行的 AI 助手平台，拥有 78 个扩展和 52 个技能。腾讯、字节、Kimi 等大厂正在为其发布高质量插件 — 微信集成、飞书机器人、搜索等。

问题在于：OpenClaw 本身太重、太复杂、有安全隐患。生产级开发者不会跑它。

**Claw2Cli** 把这些插件提取出来，以纯 CLI 命令形式暴露。底层通过 `npx` 调用，插件原生运行，无需修改。任何能调 shell 的工具 — Claude Code、Gemini CLI、Python 脚本、CI 流水线 — 都能直接使用。

## 快速开始

```bash
# 安装
go install github.com/YangZhengCQ/Claw2cli@latest

# 安装插件
c2c install @tencent-weixin/openclaw-weixin-cli --type connector

# 启动微信连接器（前台模式，显示二维码供扫码登录）
c2c connect wechat

# 查看有哪些功能
c2c call wechat --list-tools

# 调用工具
c2c call wechat wechat_send_text '{"to":"user@im.wechat","text":"hello"}'

# 或后台运行
c2c connect wechat -b

# 查看状态
c2c status

# 技能型插件示例
c2c install @some-scope/openclaw-search
c2c run search --query "AI news"
```

**`--list-tools` 输出效果 — 能力在运行时从插件自身内省获取，零硬编码：**

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

## 前置条件

- **Go 1.22+**（仅编译需要）
- **Node.js 18+** 和 npm（运行时 — 插件是 npm 包）
- **[tsx](https://github.com/privatenumber/tsx)**（首次 `connect` 时自动安装 — ESM + TypeScript 插件需要）
- **macOS** 或 **Linux**（不支持 Windows）

## 命令参考

| 命令 | 说明 |
|------|------|
| `c2c run <skill> [args]` | 运行技能插件（一次性） |
| `c2c connect <connector>` | 启动连接器（默认前台，`-b` 后台运行） |
| `c2c stop <connector>` | 停止运行中的连接器 |
| `c2c attach <connector>` | 连接到运行中的连接器，查看消息流 |
| `c2c echo <connector>` | 测试消费者，自动回复收到的消息 |
| `c2c call <connector> <tool> [json]` | 调用已发现的工具（通用 RPC） |
| `c2c call <connector> --list-tools` | 列出运行中连接器的可用工具 |
| `c2c status` | 显示连接器运行状态 |
| `c2c logs <connector> -f` | 跟踪连接器日志 |
| `c2c list` | 列出已安装插件 |
| `c2c info <plugin>` | 查看插件详情（运行中时显示已发现工具） |
| `c2c install <package>` | 安装 OpenClaw 插件 |
| `c2c mcp serve` | 启动 MCP 服务器（自动包含已发现工具） |

## 能力发现

插件暴露的能力（发送文本、发送媒体等）会被 c2c **在运行时自动发现**。Go 二进制中没有任何硬编码的插件逻辑。

```bash
# 启动连接器
c2c connect wechat

# 在另一个终端 — 查看它能做什么
c2c call wechat --list-tools
# 输出：
#   wechat_send_text     Send a text message via wechat
#     --to       Recipient ID
#     --text     Message content (max 4000 chars)
#
#   wechat_send_media    Send an image or file...
#     --to       Recipient ID
#     --media    Absolute local path or HTTPS URL

# 调用工具
c2c call wechat wechat_send_text '{"to":"user@im.wechat","text":"hello"}'
```

同样的工具会自动通过 MCP 暴露，Claude Code 等 Agent 可以直接调用。

## 配合 Claude Code 使用（MCP）

在 Claude Code MCP 设置中添加：

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

指定暴露范围：

```bash
c2c mcp serve --skills search,translate --connectors wechat
```

## 架构

```
用户 / Agent / 脚本
        |
        v
+------------------+
|    c2c CLI       |  <- Go 单二进制
+------------------+
| 命令路由器        |  <- run / connect / list / echo / mcp
+------+-----------+
| 技能  | 连接器    |
| 运行器| 管理器    |
|(pipe)| (daemon) |
+------+-----------+
| 插件 Shim        |  <- SKILL.md 解析 + tsx/npx 桥接
+------------------+
| 权限守卫         |  <- 基于 manifest 的访问控制
+------------------+
| MCP 服务器       |  <- JSON-RPC over stdio
+------------------+
        |
        v
  OpenClaw 插件（npm 包，原生运行）
```

## 插件模型

**技能型 (Skill)：** 无状态，一次性工具（搜索、翻译、代码分析）。以子进程运行，返回结果后退出。权限受限于网络访问和临时文件。

**连接型 (Connector)：** 有状态，长连接守护进程（微信、飞书、Discord）。保持后台连接，通过 Unix Domain Socket 通信（NDJSON 协议），可访问持久存储和凭证。

| | 技能 | 连接器 |
|---|---|---|
| 生命周期 | 调用并返回 | 后台守护进程 |
| 状态 | 无 | 会话持久化 |
| IPC | stdin/stdout pipe | Unix Domain Socket |
| 权限 | 受限 | 受控高权限 |

## 安全性

Claw2Cli 采用分层安全策略：

- **包完整性：** 安装时记录 SHA-512 校验和
- **权限清单：** 每个插件在 `manifest.yaml` 中声明能力；未授权操作在执行前被拦截
- **存储隔离：** 每个插件有独立数据目录（`~/.c2c/storage/<name>/`），权限 0700
- **进程隔离：** 插件以独立子进程运行（`os/exec`）

运行时沙箱（Linux seccomp-bpf、macOS sandbox-exec）计划在后续版本实现。

## 已支持插件

| 插件 | 类型 | 来源 | 工具数 | 状态 |
|------|------|------|--------|------|
| 微信 | 连接器 | `@tencent-weixin/openclaw-weixin-cli` | 2 | 全链路验证（登录、收发、MCP） |
| 飞书 | 连接器 | `@larksuite/openclaw-lark` | 27 | 加载+发现验证 |
| QQ Bot | 连接器 | `@tencent-connect/openclaw-qqbot` | 3 | 加载+发现验证 |
| 企业微信 | 连接器 | `@wecom/wecom-openclaw-plugin` | 3 | 加载+发现+网关验证 |
| Web Search | 技能 | `@ollama/openclaw-web-search` | 2 | 纯技能插件，发现验证 |

已测试 7 个插件，6 个成功加载（共 39 个工具）。原则上，任何 OpenClaw 兼容的 npm 包都可以安装——插件能力在运行时自动内省，Go 二进制中无任何硬编码的插件逻辑。

## 开发

```bash
git clone https://github.com/YangZhengCQ/Claw2cli.git
cd Claw2cli
make build    # 编译二进制
make test     # 运行测试
make lint     # 运行 go vet
make install  # 安装到 $GOPATH/bin
```

## 文档

| 文档 | 说明 |
|------|------|
| [设计文档](docs/DESIGN.md) | 架构设计、决策记录（英文） |
| [工程实施](docs/IMPLEMENTATION.md) | 具体实现细节、代码结构（英文） |
| [踩坑记录](docs/PITFALLS.md) | 开发过程中的问题和解决方案 |

## 许可证

MIT

## 致谢

由人类开发者、[Gemini](https://gemini.google.com) 和 [Claude Code](https://claude.ai/code) 三方协作开发。
