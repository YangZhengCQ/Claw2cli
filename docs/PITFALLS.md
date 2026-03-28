# 踩坑记录

> 最后更新：2026-03-28

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

---

## 8. Socket 文件消失（defer os.Remove 竞态）

**现象：** 后台模式 `c2c connect wechat -b` 启动后，`~/.c2c/sockets/wechat.sock` 在文件系统上不存在，但 `lsof` 能看到 daemon 在监听该路径。`c2c call --list-tools` 和 MCP 均连接失败。

**排查过程：**
1. `lsof -p <daemon-pid>` 显示 fd 5 绑定了 `wechat.sock` — 内核确认 socket 存在
2. `ls ~/.c2c/sockets/` 为空 — 文件系统上不可见
3. 前台模式 socket 正常可见，后台模式不行
4. 检查代码发现 `daemon.go` 有 `defer os.Remove(socketPath)`

**根因：** `StopConnector` 发 SIGTERM 给旧 daemon → `cleanupConnectorFiles` 删除旧 socket → 启动新 daemon → 新 daemon 创建 socket → **旧 daemon 进程退出时 `defer os.Remove(socketPath)` 触发 → 删掉了新 daemon 的 socket**。

时序竞态：
```
t=0  stop: SIGTERM → old daemon
t=1  stop: cleanupConnectorFiles → 删 socket（旧的）
t=2  start: 新 daemon 启动
t=5  start: 新 daemon 创建 socket（新的）✅
t=9  旧 daemon defer 触发 → os.Remove(socketPath) → 删了新 socket ❌
```

**解决方案：** 去掉 `defer os.Remove(socketPath)`。Socket 清理只由 `StopConnector.cleanupConnectorFiles` 和 daemon 启动时的 `os.Remove(socketPath)` 负责。

**教训：** detached 子进程的 `defer` 清理可能在任意时间点触发。共享文件路径的清理不要用 defer，要用显式的生命周期管理。

---

## 9. contextToken 缺失导致发送失败

**现象：** 通过 MCP `tools/call wechat_send_text` 发送消息，返回 `INVOKE_FAILED: contextToken is required`。

**排查过程：**
1. 微信插件的 `sendWeixinOutbound` 要求 `contextToken` 参数
2. `contextToken` 是从 inbound 消息中缓存的——用户发消息时，插件自动记录每个 `(accountId, to)` 的 token
3. daemon 重启后缓存丢失，没有 context token 就无法回复

**根因：** 微信 API 要求 outbound 消息携带从 inbound 获取的 context token（防止主动骚扰）。必须先收到用户消息建立上下文，才能回复。

**解决方案：** 这是微信平台的设计约束，不是 bug。用户需要先给机器人发一条消息，shim 缓存 context token 后即可正常回复。

**教训：** 即时通讯平台通常有 context/session 要求，不能无条件主动发消息。Capability Discovery 应该在 tool description 中说明这类约束。

---

## 10. accountId 未自动解析

**现象：** MCP 调用 `wechat_send_text` 只传了 `to` 和 `text`，没传 `accountId`，插件报错 `accountId is required (no default account)`。

**排查过程：**
1. `handleInvokeTool` 直接把 `args.accountId`（undefined）传给插件
2. 插件的 `resolveWeixinAccount(cfg, undefined)` 不接受空 accountId

**根因：** shim bridge 没有自动 fallback 到默认账号。

**解决方案：** 在 `handleInvokeTool` 中，如果 `args.accountId` 为空，自动从 `channel.config.listAccountIds()` 取第一个账号：
```javascript
if (!accountId && registeredChannel.config?.listAccountIds) {
  const ids = registeredChannel.config.listAccountIds(globalConfig);
  if (ids?.length > 0) accountId = ids[0];
}
```

**教训：** Bridge 函数要对调用者友好——能自动推断的参数就不要强制要求。单账号场景是最常见的 case。

---

## 11. `EnsurePluginInstalled` 缺少 `--ignore-scripts`

> 添加于 2026-03-28，由 grill report TDD 审查发现

**现象：** `store.Install()` 正确使用了 `--ignore-scripts`，但 `nodeutil.EnsurePluginInstalled()` 的 `npm install -g` 调用没有这个 flag。

**根因：** 两个独立的安装路径——本地安装（store）和全局安装（nodeutil）——分别实现，全局路径遗漏了安全 flag。

**影响：** 供应链攻击向量：恶意 npm 包可以通过 `postinstall` 脚本在全局安装时执行任意代码。

**解决方案：** 在 `nodeutil.go:148` 的 `npm install -g` 命令中添加 `--ignore-scripts` flag。

**教训：** 当同一个操作有多个代码路径时（本地 vs 全局安装），安全策略必须在所有路径中一致实施。代码重复是安全漏洞的温床。

---

## 12. `NODE_EXTRA_CA_CERTS` 通过环境变量过滤

> 添加于 2026-03-28，由 grill report 安全审查发现

**现象：** 环境变量过滤器阻止了 `NODE_OPTIONS`（防止 `--require` 注入）和 `NODE_AUTH_TOKEN`，但允许 `NODE_EXTRA_CA_CERTS` 通过。

**根因：** `NODE_EXTRA_CA_CERTS` 匹配 `NODE_` 安全前缀，但未被列入 `sensitiveEnvVars` 黑名单。

**影响：** 攻击者如果能控制父进程环境，可以注入恶意 CA 证书，对插件的所有 TLS 连接进行中间人攻击。

**解决方案：** 将 `NODE_EXTRA_CA_CERTS=` 添加到 `environment.go` 的 `sensitiveEnvVars` 黑名单中。

**教训：** 环境变量白名单（`NODE_` 前缀）+ 黑名单（`sensitiveEnvVars`）的组合方案要定期审查。新的安全敏感变量可能匹配白名单前缀。

---

## 13. empty array 在 allowlist 函数间语义不一致

> 添加于 2026-03-28，由 TDD 测试明确记录

**现象：** `checkSenderAllowlist([])` 阻止所有发送者，但 `isNormalizedSenderAllowed(id, [])` 允许所有发送者。

**根因：** 两个函数的空数组检查逻辑不同：
- `checkSenderAllowlist`：空数组是 truthy，`[].includes(x)` 永远返回 false → 阻止
- `isNormalizedSenderAllowed`：`allowList.length === 0` 时 early return true → 允许

**影响：** 插件作者使用错误的函数可能意外允许或阻止未授权访问。

**当前状态：** 测试明确记录了这个分歧（`shim/test/auth.test.js` Suite A #4 vs Suite E #3），但未修改行为（需要与上游 OpenClaw SDK 保持兼容）。

**教训：** API 设计中，相似函数的边界行为必须一致或明确记录。空集合的语义（"无限制" vs "全部阻止"）是常见的歧义来源。

---

## 14. npm 发布新版导致 checksum 不匹配

> 添加于 2026-03-28，E2E 测试时发现

**现象：** `c2c connect wechat -f` 启动失败，报错 `checksum mismatch for @tencent-weixin/openclaw-weixin-cli@latest: expected sha512-TRs..., got sha512-BTU...`。

**排查过程：**
1. manifest.yaml 中记录的 integrity hash 是安装时的值
2. `@latest` tag 指向的包内容已变（npm 发布了新版本或重新发布）
3. `GetNpmChecksum` 查询到新 hash，与 manifest 中记录的旧 hash 不匹配

**根因：** 使用 `@latest` 作为 source 时，npm registry 的 `dist.integrity` 会随版本更新而变化。这是完整性校验正确工作的表现，不是 bug。

**解决方案：** 使用 `c2c update wechat` 重新安装并更新 manifest 中的 hash。

**教训：** 完整性校验 + `@latest` tag 的组合意味着每次上游发版都需要 `c2c update`。未来可考虑将 `resolved_version` 锁定为具体版本号（如 `2.0.1`），仅在显式 update 时升级。

---

## 15. 微信长连接导致优雅关闭超时

> 添加于 2026-03-28，E2E 测试时观察到

**现象：** `c2c connect wechat -f` 收到 SIGTERM 后，shim 未在 9 秒内退出，被 SIGKILL 强制终止。日志显示 `shim did not exit in time, force killing`。

**根因：** 微信插件的 `startAccount` 启动了一个长轮询（long-poll）连接到微信服务器。收到 SIGTERM 后，`abortController.abort()` 触发，但长轮询请求可能正在 `await` 中，HTTP 连接关闭需要时间。

**当前状态：** 功能正常——SIGKILL 能确保进程退出。但 `force killing` 意味着：
- 微信 API 连接未正常断开（服务端会在超时后自动清理）
- 任何正在进行的消息发送可能丢失

**后续方向：** 可考虑增大 `shimShutdownTimeout`（从 9s 到 15s），或在 shim 中为长轮询请求设置 `AbortSignal` 的超时传播，使 HTTP 请求能更快中断。

---

## 16. Shim 测试挂起：readline 阻止 Node.js 进程退出

> 添加于 2026-03-28，CI shim test 超时问题

**现象：** CI 步骤 `node --test shim/test/*.test.js` 所有 38 个测试全部通过，但进程永不退出，2 分钟后被 CI 超时终止。

**排查过程：**
1. 所有测试结果都是 `ok`，TAP 输出完整
2. `node --test` 等待事件循环清空才退出
3. SDK 模块在 `require()` 时执行 `createInterface({ input: process.stdin })`
4. readline 在 stdin 上注册了活跃监听器，阻止事件循环退出

**根因：** `createInterface()` 创建的 readline 接口让 Node.js 认为还有活跃的 I/O 操作（stdin 监听），因此不会退出。在生产环境中，gateway 循环和 signal handler 负责保持进程存活；但在测试中，测试完成后没有其他工作，进程应该退出但被 readline 阻止了。

**解决方案：**
```javascript
const rl = createInterface({ input: process.stdin, terminal: false });
if (process.stdin._handle && typeof process.stdin._handle.unref === "function") {
  process.stdin._handle.unref();
}
```

`unref()` 告诉 Node.js 这个句柄单独不应阻止进程退出。效果：
- 测试中：测试完成后进程正常退出（<1s）
- 生产中：gateway 循环 + signal handler 保持进程存活，unref 无影响

**注意：** `process.stdin.unref()` 不存在（stdin 是 `ReadStream` 不是 `Socket`），必须用 `process.stdin._handle.unref()` 访问底层句柄。

**教训：** `readline.createInterface()` 会隐式地将 stdin 标记为"活跃"，阻止进程退出。任何在模块顶层创建 readline 的库在测试环境中都会遇到这个问题。`_handle.unref()` 是标准的 Node.js 解决方案。
