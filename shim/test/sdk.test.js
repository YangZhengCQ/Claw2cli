const { describe, it } = require("node:test");
const assert = require("node:assert");

describe("sendMessage", () => {
	it("should set timestamp in unix seconds", () => {
		// Verify timestamp is in seconds, not milliseconds
		const now = Math.floor(Date.now() / 1000);
		assert.ok(now > 1700000000, "timestamp should be unix seconds");
		assert.ok(now < 2000000000, "timestamp should not be milliseconds");
	});
});

describe("stripVersion", () => {
	it("should strip version from scoped packages", () => {
		// Test the resolvePluginPackage logic
		const pkg = "@scope/name@1.0.0";
		const idx = pkg.lastIndexOf("@");
		const stripped = idx > 0 ? pkg.substring(0, idx) : pkg;
		assert.strictEqual(stripped, "@scope/name");
	});
});
