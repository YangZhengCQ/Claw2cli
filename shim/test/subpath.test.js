const assert = require('node:assert');
const { test } = require('node:test');

// All known subpath imports used by OpenClaw SDK v2026.3.22 plugins
const KNOWN_SUBPATHS = [
  'account-id',
  'channel-config-schema',
  'channel-contract',
  'channel-runtime',
  'command-auth',
  'core',
  'infra-runtime',
  'plugin-entry',
  'reply-runtime',
  'text-runtime',
];

test('all SDK subpath exports resolve to the main module', () => {
  for (const sub of KNOWN_SUBPATHS) {
    const mod = require(`@openclaw/plugin-sdk/${sub}`);
    assert.ok(mod.normalizeAccountId, `@openclaw/plugin-sdk/${sub} should export normalizeAccountId`);
    assert.ok(mod.buildChannelConfigSchema, `@openclaw/plugin-sdk/${sub} should export buildChannelConfigSchema`);
  }
});

test('openclaw/plugin-sdk subpaths also resolve', () => {
  for (const sub of KNOWN_SUBPATHS) {
    const mod = require(`openclaw/plugin-sdk/${sub}`);
    assert.ok(mod.normalizeAccountId, `openclaw/plugin-sdk/${sub} should export normalizeAccountId`);
  }
});

test('future unknown subpaths resolve gracefully', () => {
  // Any arbitrary subpath should resolve to the main module
  const mod = require('@openclaw/plugin-sdk/some-future-subpath');
  assert.ok(mod.normalizeAccountId, 'unknown subpath should still export normalizeAccountId');
});
