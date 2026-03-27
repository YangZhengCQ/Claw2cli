const { describe, it, beforeEach } = require("node:test");
const assert = require("node:assert");
const sdk = require("@openclaw/plugin-sdk");

const setConfig = sdk._internal.setGlobalConfig;

// Suite A: resolveSenderCommandAuthorization — checkSenderAllowlist via globalConfig
describe("resolveSenderCommandAuthorization", () => {
	beforeEach(() => setConfig({}));

	it("allows any sender when authorized_senders is undefined", () => {
		setConfig({});
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.strictEqual(result.authorized, true);
	});

	it("allows any sender when authorized_senders is null", () => {
		setConfig({ authorized_senders: null });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.strictEqual(result.authorized, true);
	});

	it("allows any sender when authorized_senders is non-array", () => {
		setConfig({ authorized_senders: "u1" });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.strictEqual(result.authorized, true);
	});

	it("blocks all senders when authorized_senders is empty array", () => {
		setConfig({ authorized_senders: [] });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.strictEqual(result.authorized, false);
	});

	it("allows listed sender", () => {
		setConfig({ authorized_senders: ["u1", "u2"] });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.strictEqual(result.authorized, true);
	});

	it("blocks unlisted sender", () => {
		setConfig({ authorized_senders: ["u1"] });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u2" });
		assert.strictEqual(result.authorized, false);
	});
});

// Suite B: senderId extraction priority
describe("senderId extraction priority", () => {
	beforeEach(() => setConfig({ authorized_senders: ["a", "b", "c"] }));

	it("prefers ctx.senderId over ctx.from and ctx.From", () => {
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "a", from: "b", From: "c" });
		assert.strictEqual(result.authorized, true);
	});

	it("falls back to ctx.from when senderId is absent", () => {
		const result = sdk.resolveSenderCommandAuthorization({ from: "b", From: "c" });
		assert.strictEqual(result.authorized, true);
	});

	it("falls back to ctx.From as last resort", () => {
		const result = sdk.resolveSenderCommandAuthorization({ From: "c" });
		assert.strictEqual(result.authorized, true);
	});

	it("senderId takes precedence even when from is in allowlist", () => {
		setConfig({ authorized_senders: ["b"] });
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "a", from: "b" });
		assert.strictEqual(result.authorized, false);
	});
});

// Suite C: resolveSenderCommandAuthorizationWithRuntime
describe("resolveSenderCommandAuthorizationWithRuntime", () => {
	beforeEach(() => setConfig({}));

	it("allows listed sender", () => {
		setConfig({ authorized_senders: ["u1"] });
		const result = sdk.resolveSenderCommandAuthorizationWithRuntime({}, { senderId: "u1" });
		assert.strictEqual(result.authorized, true);
	});

	it("blocks unlisted sender", () => {
		setConfig({ authorized_senders: ["u1"] });
		const result = sdk.resolveSenderCommandAuthorizationWithRuntime({}, { senderId: "u2" });
		assert.strictEqual(result.authorized, false);
	});

	it("runtime param does not affect authorization", () => {
		setConfig({ authorized_senders: ["u1"] });
		const r1 = sdk.resolveSenderCommandAuthorizationWithRuntime(null, { senderId: "u1" });
		const r2 = sdk.resolveSenderCommandAuthorizationWithRuntime({ key: "val" }, { senderId: "u1" });
		const r3 = sdk.resolveSenderCommandAuthorizationWithRuntime(42, { senderId: "u1" });
		assert.strictEqual(r1.authorized, true);
		assert.strictEqual(r2.authorized, true);
		assert.strictEqual(r3.authorized, true);
	});
});

// Suite D: resolveDirectDmAuthorizationOutcome — returns { allowed } not { authorized }
describe("resolveDirectDmAuthorizationOutcome", () => {
	beforeEach(() => setConfig({}));

	it("allows when authorized_senders is undefined", () => {
		setConfig({});
		const result = sdk.resolveDirectDmAuthorizationOutcome({ senderId: "u1" }, {});
		assert.strictEqual(result.allowed, true);
	});

	it("blocks when authorized_senders is empty array", () => {
		setConfig({ authorized_senders: [] });
		const result = sdk.resolveDirectDmAuthorizationOutcome({ senderId: "u1" }, {});
		assert.strictEqual(result.allowed, false);
	});

	it("allows listed sender", () => {
		setConfig({ authorized_senders: ["u1"] });
		const result = sdk.resolveDirectDmAuthorizationOutcome({ senderId: "u1" }, {});
		assert.strictEqual(result.allowed, true);
	});

	it("blocks unlisted sender", () => {
		setConfig({ authorized_senders: ["u1"] });
		const result = sdk.resolveDirectDmAuthorizationOutcome({ senderId: "u2" }, {});
		assert.strictEqual(result.allowed, false);
	});

	it("config param does NOT override globalConfig", () => {
		setConfig({}); // no authorized_senders => allow all
		const result = sdk.resolveDirectDmAuthorizationOutcome(
			{ senderId: "u1" },
			{ authorized_senders: ["other"] }, // this should be ignored
		);
		assert.strictEqual(result.allowed, true);
	});
});

// Suite E: isNormalizedSenderAllowed — explicit allowList param, no globalConfig
describe("isNormalizedSenderAllowed", () => {
	it("allows when allowList is null", () => {
		assert.strictEqual(sdk.isNormalizedSenderAllowed("u1", null), true);
	});

	it("allows when allowList is undefined", () => {
		assert.strictEqual(sdk.isNormalizedSenderAllowed("u1", undefined), true);
	});

	// NOTE: empty array allows all — diverges from checkSenderAllowlist which blocks all
	it("allows when allowList is empty array (diverges from globalConfig behavior)", () => {
		assert.strictEqual(sdk.isNormalizedSenderAllowed("u1", []), true);
	});

	it("allows listed sender", () => {
		assert.strictEqual(sdk.isNormalizedSenderAllowed("u1", ["u1", "u2"]), true);
	});

	it("blocks unlisted sender", () => {
		assert.strictEqual(sdk.isNormalizedSenderAllowed("u3", ["u1", "u2"]), false);
	});
});

// Suite F: Return shape contracts
describe("return shape contracts", () => {
	beforeEach(() => setConfig({}));

	it("resolveSenderCommandAuthorization returns only authorized key", () => {
		const result = sdk.resolveSenderCommandAuthorization({ senderId: "u1" });
		assert.deepStrictEqual(Object.keys(result), ["authorized"]);
	});

	it("resolveDirectDmAuthorizationOutcome returns only allowed key", () => {
		const result = sdk.resolveDirectDmAuthorizationOutcome({ senderId: "u1" }, {});
		assert.deepStrictEqual(Object.keys(result), ["allowed"]);
	});
});
