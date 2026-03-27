const { describe, it } = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const path = require("path");

// --- Test 1: rl.on('close') rejects pending requests ---
describe("pending request cleanup on stdin close", () => {
	it("exposes pendingRequests Map via _internal", () => {
		const sdk = require("@openclaw/plugin-sdk");
		const pending = sdk._internal.getPendingRequests();
		assert.ok(pending instanceof Map, "pendingRequests should be a Map");
	});

	it("has a close handler on the readline interface", () => {
		const sdk = require("@openclaw/plugin-sdk");
		const rl = sdk._internal.getRl();
		const closeListeners = rl.rawListeners("close");
		assert.ok(
			closeListeners.length > 0,
			"readline should have a 'close' event listener to clean up pending requests",
		);
	});

	it("close handler source references pendingRequests and reject", () => {
		// Verify the close handler iterates pending requests and rejects them.
		// We inspect the handler source rather than calling it, to avoid mutating
		// module state (stdoutClosed) which would break subsequent tests.
		const sdk = require("@openclaw/plugin-sdk");
		const rl = sdk._internal.getRl();
		const closeHandler = rl.rawListeners("close")[0];
		const src = closeHandler.toString();

		assert.ok(src.includes("pendingRequests"), "close handler should reference pendingRequests");
		assert.ok(src.includes("reject"), "close handler should reject pending requests");
		assert.ok(src.includes("clear"), "close handler should clear the pendingRequests map");
	});

	it("SDK source has rl.on('close') with stdin closed message", () => {
		const sdkSource = fs.readFileSync(
			path.join(__dirname, "..", "node_modules", "@openclaw", "plugin-sdk", "index.js"),
			"utf-8",
		);
		assert.ok(
			sdkSource.includes('rl.on("close"'),
			"SDK should register rl.on('close') handler",
		);
		assert.ok(
			sdkSource.includes("stdin closed"),
			"close handler should mention stdin closed in rejection message",
		);
	});
});

// --- Test 2: listAccounts error logging ---
describe("listAccounts error handling", () => {
	it("catch block logs error via sendLog (not silent)", () => {
		const shimSource = fs.readFileSync(
			path.join(__dirname, "..", "c2c-shim.js"),
			"utf-8",
		);

		// The catch block around listAccountIds should contain sendLog
		const catchMatch = shimSource.match(
			/listAccountIds[\s\S]*?catch\s*\(e\)\s*\{([^}]+)\}/,
		);
		assert.ok(catchMatch, "should have a catch block around listAccountIds");
		assert.ok(
			catchMatch[1].includes("sendLog"),
			"catch block should call sendLog to report the error",
		);
	});
});

// --- Test 3: unhandledRejection handler ---
describe("unhandledRejection handler", () => {
	it("c2c-shim.js registers process.on('unhandledRejection')", () => {
		const shimSource = fs.readFileSync(
			path.join(__dirname, "..", "c2c-shim.js"),
			"utf-8",
		);

		assert.ok(
			shimSource.includes('process.on("unhandledRejection"'),
			"c2c-shim.js should register an unhandledRejection handler",
		);
	});

	it("unhandledRejection handler calls sendLog", () => {
		const shimSource = fs.readFileSync(
			path.join(__dirname, "..", "c2c-shim.js"),
			"utf-8",
		);

		const handlerMatch = shimSource.match(
			/process\.on\("unhandledRejection"[\s\S]*?\}\);/,
		);
		assert.ok(handlerMatch, "should have an unhandledRejection handler");
		assert.ok(
			handlerMatch[0].includes("sendLog"),
			"unhandledRejection handler should call sendLog",
		);
	});
});

// --- Test 4: SDK error handling separates JSON parse from handler errors ---
describe("SDK rl.on('line') error handling", () => {
	it("separates JSON parse errors from handler errors in catch block", () => {
		const sdkSource = fs.readFileSync(
			path.join(__dirname, "..", "node_modules", "@openclaw", "plugin-sdk", "index.js"),
			"utf-8",
		);

		assert.ok(
			sdkSource.includes("SyntaxError"),
			"catch block should check for SyntaxError to distinguish parse vs handler errors",
		);
	});
});
