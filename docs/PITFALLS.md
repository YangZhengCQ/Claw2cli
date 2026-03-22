# 踩坑记录

> 最后更新：2026-03-23

记录 Claw2Cli 开发过程中遇到的关键问题、排查过程和解决方案。

---

## 1. ESM + TypeScript：Cannot find module

**现象：** `c2c connect wechat` 启动后 shim 立即退出，报错 `Cannot find module '@tencent-weixin/openclaw-weixin'`。

**排查过程：**
1. 确认 NODE_PATH 设置正确，包确实在 `/opt/homebrew/lib/node_modules/` 下
2. 直接用 `node -e "require.resolve('@tencent-weixin/openclaw-weixin')"` 测试 → 同样失败
3. 检查 `package.json`：`"type": "module"`，入口是 `index.ts`（TypeScript），无 `main`/`exports` 字段
4. 发现大厂插件发布的是 **ESM + TypeScript 源码**，没有预编译的 JS

**根因：** Node.js 的 `require()` 无法加载 ESM 模块，也不能直接执行 `.ts` 文件。

**解决方案：**
- 用 [tsx](https://github.com/privatenumber/tsx) 替代 `node` 运行 shim — tsx 内置 TypeScript 编译 + ESM 支持
- shim 中把 `require(pkg)` 改为 `await import(pkg)`，并保留 `require()` 作为 CJS 兜底
- `resolveNodeRunner()` 优先找 tsx，找不到就自动 `npm install -g tsx`

**教训：** 不要假设 npm 包都发布了编译好的 JS。大厂插件（尤其是 OpenClaw 生态内的）可能只发布 TS 源码，依赖宿主环境的 TypeScript 加载能力。

---

## 2. CLI 包 ≠ 运行时包

**现象：** 安装了 `@tencent-weixin/openclaw-weixin-cli`，但 shim 加载 `@tencent-weixin/openclaw-weixin` 失败。

**排查过程：**
1. `npm list -g` 只有 `openclaw-weixin-cli`，没有 `openclaw-weixin`
2. 读 CLI 包源码（`cli.mjs`）：它只是个安装器，检查 openclaw 是否存在，然后调 `openclaw plugins install`
3. 实际的插件运行时是 **另一个包**，去掉 `-cli` 后缀

**根因：** 部分 OpenClaw 插件拆成两个 npm 包 — CLI 安装器（`*-cli`）和运行时（无 `-cli`）。

**解决方案：**
- `resolvePluginPackage()` 自动去掉 `-cli` 后缀和版本号
- `install` 和 `daemon` 都同时安装两个包
- shim 的 `resolvePluginPackage()` 也做同样的去 `-cli` 处理

**教训：** npm 包名不等于模块名。安装器和运行时可能是不同的包。

---

## 3. QR 码不显示

**现象：** `c2c connect wechat` 启动后日志正常，但看不到微信扫码二维码。

**排查过程：**
1. 手动运行 `tsx c2c-shim.js wechat`（不通过 daemon）→ 二维码正常显示在 stdout
2. 通过 daemon 运行 → stdout 全部被 NDJSON 解析器吞掉
3. 二维码是插件直接 `console.log` 打到 stdout 的字符画，不是 NDJSON

**根因：**
- daemon 把 stdout 全部 pipe 走做 NDJSON 解析，非 JSON 行被静默丢弃
- daemon 把 stderr 也捕获为 NDJSON log 消息，不透传到终端

**解决方案：**
- 加前台模式（`isForeground`）：stderr 直通终端，stdout 中非 JSON 行也打到终端
- 默认 `c2c connect` 前台运行，`-b` 切后台
- 后台模式下这些输出进日志文件，不丢失

**教训：** 第三方插件的输出不一定走你设计的协议。要考虑 raw stdout/stderr 输出（二维码、进度条等）。

---

## 4. EPIPE 导致进程崩溃

**现象：** Ctrl+C 停止连接器时，shim 报 `Error: write EPIPE` 未处理异常并崩溃。

**排查过程：**
1. 时序：Go 收到 SIGINT → 发 SIGTERM 给 shim → 关闭 stdout pipe
2. shim 收到 SIGTERM → 处理中 → 某个异步回调尝试 `sendEvent()` → stdout 已断 → EPIPE
3. Node.js 默认 EPIPE 是未处理错误 → 进程崩溃

**根因：** Go 侧关闭 pipe 后，shim 的异步回调（如 `setStatus`）可能仍在尝试写 stdout。

**解决方案：**
```javascript
process.stdout.on("error", (err) => {
  if (err.code === "EPIPE" || err.code === "ERR_STREAM_DESTROYED") {
    stdoutClosed = true;
  }
});

function sendMessage(msg) {
  if (stdoutClosed) return;  // 静默跳过
  // ...
}
```

**教训：** 跨进程 pipe 通信必须处理写端关闭。不要假设 stdout 永远可写。

---

## 5. 前台模式无 PID 文件导致 attach/echo 失败

**现象：** 前台运行 `c2c connect wechat` 后，另一个终端 `c2c echo wechat` 报错 `no such file or directory: wechat.pid`。

**排查过程：**
1. 前台模式直接调 `runDaemon()`，不经过 `executor.StartConnector()`
2. `StartConnector` 创建 PID 文件，`runDaemon` 不创建
3. `AttachConnector` 先读 PID 文件检查进程状态 → 失败

**根因：** `AttachConnector` 强依赖 PID 文件，但前台模式没有 PID 文件。

**解决方案：**
```go
func AttachConnector(name string) (net.Conn, error) {
    // 先尝试 PID 方式（后台模式）
    status, err := GetConnectorStatus(name)
    if err == nil && status.Running {
        return netDial("unix", status.Socket)
    }
    // 兜底：直连 socket 文件（前台模式）
    return netDial("unix", paths.SocketPath(name))
}
```

**教训：** 不要假设所有运行模式都有相同的状态文件。提供降级路径。

---

## 6. foregroundMode 变量名反了

**现象：** 代码审计发现 `var foregroundMode bool` 由 `--background/-b` flag 设置，`if foregroundMode` 执行的是后台逻辑。

**排查过程：** 变量叫 `foregroundMode`，flag 叫 `background`，赋值给 `foregroundMode`。当用户传 `-b` 时 `foregroundMode = true`，进入 `if foregroundMode` 分支执行后台模式。逻辑能跑通但语义完全反了。

**根因：** 最初写的时候从 "是否前台" 的角度命名，后来改为 "后台是特殊模式" 但忘了改变量名。

**解决方案：** 重命名为 `backgroundMode`。

**教训：** flag 变量名要跟 flag 名一致。语义反转的布尔变量是 bug 温床。

---

## 7. get_reply 阻塞后续消息

**现象：** 收到第一条微信消息后 5 分钟内，第二条消息不会触发 `message.received` 事件。

**排查过程：**
1. shim 的 `dispatchReplyFromConfig` 调用 `sendCommand("get_reply", ...)` 并 `await` 等待
2. 当前没有消费者回复这个 command → 5 分钟超时
3. 超时期间函数阻塞，插件的消息处理链也阻塞

**根因：** `get_reply` 是同步等待模式，无人回复就阻塞。

**当前状态：** 已通过 `c2c echo` 测试消费者验证了回复链路。echo 连接后 `get_reply` 立即得到回复，不再阻塞。

**后续方向：** 需要真正的消费者（MCP 客户端 / LLM 调用）来处理 `get_reply`。daemon 本身是透明管道，不应该内置回复逻辑。
