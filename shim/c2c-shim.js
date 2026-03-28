#!/usr/bin/env node
/**
 * Claw2Cli Plugin Shim Entry Point
 *
 * Usage: node c2c-shim.js <plugin-name>
 *
 * This script loads an OpenClaw plugin using the fake @openclaw/plugin-sdk,
 * registers it, starts the gateway (long-poll loop), and bridges all
 * communication to the Go daemon via stdin/stdout NDJSON.
 *
 * Environment variables:
 *   C2C_PLUGIN_NAME   - plugin name (e.g., "wechat")
 *   C2C_STORAGE_DIR   - plugin storage directory
 *   C2C_PLUGIN_SOURCE - npm package specifier
 *   C2C_ACCOUNTS      - JSON array of account configs (optional)
 */

const path = require("path");
const fs = require("fs");
const sdk = require("@openclaw/plugin-sdk");
const { PluginApiShim, sendEvent, sendLog, sendMessage, getRegisteredChannel, getGlobalRuntime, setGlobalConfig, emitToolDiscovery } = sdk._internal;

// Re-ref stdin for production: the SDK unrefs it (for test compatibility),
// but the shim needs stdin alive to receive tool invocations and daemon commands.
if (process.stdin._handle && typeof process.stdin._handle.ref === "function") {
  process.stdin._handle.ref();
}

const pluginName = process.argv[2] || process.env.C2C_PLUGIN_NAME || "unknown";
const pluginSource = process.env.C2C_PLUGIN_SOURCE || "";
const storageDir = process.env.C2C_STORAGE_DIR || "";

// ---------------------------------------------------------------------------
// Load config from storage dir
// ---------------------------------------------------------------------------

function loadConfig() {
  const configPath = path.join(storageDir, "config.json");
  if (fs.existsSync(configPath)) {
    try {
      return JSON.parse(fs.readFileSync(configPath, "utf-8"));
    } catch (e) {
      sendLog(pluginName, "warn", `Failed to load config: ${e.message}`);
    }
  }
  // Default config
  return {
    channels: {
      "openclaw-weixin": {
        baseUrl: "https://ilinkai.weixin.qq.com",
        cdnBaseUrl: "https://novac2c.cdn.weixin.qq.com/c2c",
      },
      "openclaw-lark": {},
    },
  };
}

// ---------------------------------------------------------------------------
// Load and register the plugin
// ---------------------------------------------------------------------------

// Wait for SIGTERM/SIGINT — used by skill-only plugins that have no gateway loop
function waitForShutdown() {
  return new Promise((resolve) => {
    process.on("SIGTERM", () => { sendLog(pluginName, "info", "Received SIGTERM, shutting down..."); resolve(); });
    process.on("SIGINT", () => { sendLog(pluginName, "info", "Received SIGINT, shutting down..."); resolve(); });
  });
}

// Build a hybrid logger: callable as function + has .debug/.info/.warn/.error methods
function buildAccountLogger(accountId) {
  const fn = (msg) => sendLog(pluginName, "info", `[${accountId}] ${msg}`);
  fn.debug = (msg) => sendLog(pluginName, "debug", `[${accountId}] ${msg}`);
  fn.info = fn;
  fn.warn = (msg) => sendLog(pluginName, "warn", `[${accountId}] ${msg}`);
  fn.error = (msg) => sendLog(pluginName, "error", `[${accountId}] ${msg}`);
  return fn;
}

async function main() {
  sendLog(pluginName, "info", `c2c-shim starting for ${pluginName} (${pluginSource})`);

  // Load config
  const config = loadConfig();
  setGlobalConfig(config);

  // Resolve the actual plugin module
  // OpenClaw plugins are ESM + TypeScript — use dynamic import()
  let pluginModule;
  try {
    const pluginPkgName = resolvePluginPackage(pluginSource);
    try {
      // First try dynamic import (works for ESM and with tsx loader)
      pluginModule = await import(pluginPkgName);
    } catch (importErr) {
      // Fallback to require for CJS plugins
      pluginModule = require(pluginPkgName);
    }
  } catch (e) {
    sendLog(pluginName, "error", `Failed to load plugin: ${e.message}`);
    sendMessage({ type: "error", source: pluginName, code: "LOAD_FAILED", message: e.message });
    process.exit(1);
  }

  // Get the default export (the plugin object)
  const plugin = pluginModule.default || pluginModule;

  if (!plugin || !plugin.register) {
    sendLog(pluginName, "error", "Plugin does not export a register() function");
    process.exit(1);
  }

  // Create the fake API and register
  const api = new PluginApiShim();
  plugin.register(api);

  // Emit OAPI/MCP tools collected during register() (after channel discovery)
  emitToolDiscovery();

  const channel = getRegisteredChannel();
  if (!channel) {
    // Skill-only plugin (tools registered but no channel) — stay alive for tool invocations
    sendLog(pluginName, "info", "No channel registered (skill-only plugin). Waiting for tool calls...");
    await waitForShutdown();
    return;
  }

  sendLog(pluginName, "info", `Channel ${channel.id} registered`);

  // Check for existing accounts
  let accounts = listAccounts(channel, config);
  if (accounts.length === 0) {
    sendLog(pluginName, "info", "No accounts configured, starting QR login...");
    await startLogin(channel, config);
    // Re-fetch accounts after login so newly-authenticated accounts are picked up
    accounts = listAccounts(channel, config);
    if (accounts.length === 0) {
      sendLog(pluginName, "warn", "No accounts available after login. Gateway will not start.");
    }
  }

  // Start the gateway for each account
  const runtime = getGlobalRuntime();
  const abortController = new AbortController();

  process.on("SIGTERM", () => {
    sendLog(pluginName, "info", "Received SIGTERM, shutting down...");
    abortController.abort();
  });
  process.on("SIGINT", () => {
    sendLog(pluginName, "info", "Received SIGINT, shutting down...");
    abortController.abort();
  });

  for (const accountId of accounts) {
    const account = channel.config.resolveAccount(config, accountId);
    if (!account || !channel.config.isConfigured(account)) {
      sendLog(pluginName, "warn", `Account ${accountId} not configured, skipping`);
      continue;
    }

    sendLog(pluginName, "info", `Starting account: ${accountId}`);
    sendEvent(pluginName, "account.starting", { accountId });

    if (channel.gateway && channel.gateway.startAccount) {
      channel.gateway.startAccount({
        account,
        cfg: config,
        runtime: runtime.channel,
        abortSignal: abortController.signal,
        setStatus: (status) => {
          sendEvent(pluginName, "account.status", { accountId, status });
        },
        log: buildAccountLogger(accountId),
      }).catch((err) => {
        sendLog(pluginName, "error", `Account ${accountId} error: ${err.message}`);
        sendEvent(pluginName, "account.error", { accountId, error: err.message });
      });
    }
  }

  sendEvent(pluginName, "gateway.started", { accounts });
  sendLog(pluginName, "info", "Gateway running. Waiting for messages...");
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function resolvePluginPackage(source) {
  // Strip version suffix for require
  let pkg = source;
  if (pkg.startsWith("@") && pkg.lastIndexOf("@") > 0) {
    pkg = pkg.substring(0, pkg.lastIndexOf("@"));
  } else if (!pkg.startsWith("@") && pkg.includes("@")) {
    pkg = pkg.substring(0, pkg.indexOf("@"));
  }
  // The CLI package is just an installer; we need the actual plugin
  // @tencent-weixin/openclaw-weixin-cli -> @tencent-weixin/openclaw-weixin
  // @larksuite/openclaw-lark -> @larksuite/openclaw-lark (already the plugin)
  pkg = pkg.replace(/-cli$/, "");
  return pkg;
}

function listAccounts(channel, config) {
  if (channel.config && channel.config.listAccountIds) {
    try {
      return channel.config.listAccountIds(config);
    } catch (e) {
      sendLog(pluginName, "warn", `Failed to list accounts (listAccountIds threw): ${e.message}`);
      return [];
    }
  }
  return [];
}

async function startLogin(channel, config) {
  if (!channel.auth || !channel.auth.login) {
    sendLog(pluginName, "warn", "Plugin does not support interactive login");
    return;
  }

  const runtime = getGlobalRuntime();
  try {
    await channel.auth.login({
      cfg: config,
      accountId: "default",
      verbose: true,
      runtime: runtime.channel,
    });
    sendLog(pluginName, "info", "Login completed");
    sendEvent(pluginName, "auth.login_complete", {});
  } catch (e) {
    sendLog(pluginName, "error", `Login failed: ${e.message}`);
    sendEvent(pluginName, "auth.login_failed", { error: e.message });
  }
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

process.on("unhandledRejection", (reason) => {
  const msg = reason instanceof Error ? `${reason.message}\n${reason.stack}` : String(reason);
  sendLog(pluginName, "error", `Unhandled rejection: ${msg}`);
});

main().catch((err) => {
  sendLog(pluginName, "error", `Fatal: ${err.message}\n${err.stack}`);
  process.exit(1);
});
