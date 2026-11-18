import test from 'node:test';
import assert from 'node:assert/strict';
import { isFreeTab, canUseWidget } from '../src/utils/feature-access.js';
import { getChartOwner, syncChartOwner, STORAGE_KEY_BIRTH_DATA, STORAGE_KEY_NATAL_CHART } from '../src/utils/birthData.js';

test('all requested personal and social entry points are free', () => {
  for (const tab of ['market', 'personal', 'kabbalah', 'backtest', 'dashboard', 'mycalendar']) assert.equal(isFreeTab(tab), true, tab);
  for (const tab of ['crypto', 'commodities', 'comparison', 'custom', 'clanker', 'manual-trade', 'astromap', 'upgrade', 'unknown']) assert.equal(isFreeTab(tab), false, tab);
});

test('old paid dashboard preferences cannot surface asset or PnL widgets after expiry', () => {
  const savedWidgets = ['personal-today', 'asset', 'transits', 'pnl', 'kabbalah-basics'];
  assert.deepEqual(savedWidgets.filter(type => canUseWidget(type, false)), ['personal-today', 'transits', 'kabbalah-basics']);
  assert.deepEqual(savedWidgets.filter(type => canUseWidget(type, true)), savedWidgets);
  assert.equal(canUseWidget('unknown-widget', false), false);
});

test('personal chart caches never cross account boundaries, including unowned legacy data', () => {
  const storage = new Map<string, string>();
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
  } });
  try {
    storage.set(STORAGE_KEY_BIRTH_DATA, 'unowned legacy birth');
    assert.equal(syncChartOwner('alice'), true);
    assert.equal(storage.has(STORAGE_KEY_BIRTH_DATA), false);
    assert.equal(syncChartOwner('guest'), true);
    storage.set(STORAGE_KEY_BIRTH_DATA, 'guest birth');
    assert.equal(syncChartOwner('guest'), false);
    assert.equal(storage.get(STORAGE_KEY_BIRTH_DATA), 'guest birth');
    syncChartOwner('alice');
    assert.equal(storage.has(STORAGE_KEY_BIRTH_DATA), false);
    assert.equal(getChartOwner(), 'alice');
    storage.set(STORAGE_KEY_BIRTH_DATA, 'alice birth');
    storage.set(STORAGE_KEY_NATAL_CHART, 'alice chart');
    assert.equal(syncChartOwner('alice'), false);
    assert.equal(storage.get(STORAGE_KEY_BIRTH_DATA), 'alice birth');
    assert.equal(syncChartOwner('bob'), true);
    assert.equal(storage.has(STORAGE_KEY_BIRTH_DATA), false);
    assert.equal(storage.has(STORAGE_KEY_NATAL_CHART), false);
    assert.equal(getChartOwner(), 'bob');
    storage.set(STORAGE_KEY_BIRTH_DATA, 'bob birth');
    syncChartOwner(null);
    assert.equal(getChartOwner(), null);
    assert.equal(storage.has(STORAGE_KEY_BIRTH_DATA), false);
  } finally {
    if (previous) Object.defineProperty(globalThis, 'localStorage', previous);
    else Reflect.deleteProperty(globalThis, 'localStorage');
  }
});
