import test from 'node:test';
import assert from 'node:assert/strict';
import { setImmediate } from 'node:timers/promises';
import { clearPendingPayment, markPaymentStarted, watchPaymentAccess } from '../src/utils/payment-access-watch.ts';

function browser(search = '') {
  const storage = new Map<string, string>();
  const timers = new Map<number, () => void>();
  const window = Object.assign(new EventTarget(), {
    location: { search },
    setInterval: (callback: () => void) => { const id = timers.size + 1; timers.set(id, callback); return id; },
    clearInterval: (id: number) => { timers.delete(id); },
  });
  const document = Object.assign(new EventTarget(), { visibilityState: 'visible' });
  const previous = ['window', 'document', 'localStorage'].map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)] as const);
  Object.defineProperties(globalThis, {
    window: { configurable: true, value: window },
    document: { configurable: true, value: document },
    localStorage: { configurable: true, value: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    } },
  });
  return { window, document, storage, timers,
    tick: async () => { [...timers.values()].forEach(callback => callback()); await setImmediate(); },
    restore: () => {
      for (const [key, descriptor] of previous) {
        if (descriptor) Object.defineProperty(globalThis, key, descriptor);
        else Reflect.deleteProperty(globalThis, key);
      }
    },
  };
}

test('a pending checkout automatically refreshes access after confirmation, independent of its modal', async () => {
  const b = browser();
  let status = 'none', reads = 0, refreshes = 0;
  const stop = watchPaymentAccess('alice', async () => { reads++; return { status, expiresAt: null }; }, async () => { refreshes++; });
  try {
    await b.tick();
    assert.equal(reads, 0, 'idle free accounts must not continuously poll');
    markPaymentStarted('alice');
    await setImmediate();
    assert.equal(reads, 1);
    assert.equal(refreshes, 0);
    status = 'active';
    await b.tick();
    assert.equal(refreshes, 1);
    assert.equal(b.storage.size, 0);
    assert.equal(b.timers.size, 0);
    await b.tick();
    assert.equal(refreshes, 1);
  } finally { stop(); b.restore(); }
});

test('pending checkout survives a page reload and temporary network failures', async () => {
  const b = browser();
  let attempts = 0, refreshes = 0;
  markPaymentStarted('alice');
  const stop = watchPaymentAccess('alice', async () => {
    if (++attempts === 1) throw new Error('offline');
    return { status: 'active', expiresAt: new Date(Date.now() + 60000).toISOString() };
  }, async () => { refreshes++; });
  try {
    await setImmediate();
    assert.equal(refreshes, 0);
    assert.equal(b.storage.size, 1);
    await b.tick();
    assert.equal(refreshes, 1);
  } finally { stop(); b.restore(); }
});

test('a success URL cannot grant access without a current active server membership', async () => {
  const b = browser('?payment=success&provider=nowpayments');
  let membership = { status: 'none', expiresAt: null as string | null }, refreshes = 0;
  const stop = watchPaymentAccess('alice', async () => membership, async () => { refreshes++; });
  try {
    await setImmediate();
    assert.equal(refreshes, 0);
    membership = { status: 'active', expiresAt: '2000-01-01T00:00:00Z' };
    await b.tick();
    membership.expiresAt = 'invalid';
    await b.tick();
    assert.equal(refreshes, 0);
    membership.expiresAt = new Date(Date.now() + 60000).toISOString();
    await b.tick();
    assert.equal(refreshes, 1);
  } finally { stop(); b.restore(); }
});

test('pending checkout state never follows another signed-in account', async () => {
  const b = browser();
  markPaymentStarted('alice');
  let reads = 0;
  const stop = watchPaymentAccess('bob', async () => { reads++; return { status: 'none', expiresAt: null }; }, async () => { assert.fail('unexpected activation'); });
  try {
    markPaymentStarted('alice');
    clearPendingPayment('bob');
    await b.tick();
    assert.equal(reads, 0);
    assert.equal(b.storage.size, 1);
  } finally { stop(); b.restore(); }
});

test('hidden tabs pause polling and returning to the app refreshes paid access', async () => {
  const b = browser();
  b.document.visibilityState = 'hidden';
  markPaymentStarted('alice');
  let reads = 0, refreshes = 0;
  const stop = watchPaymentAccess('alice', async () => { reads++; return { status: 'active', expiresAt: null }; }, async () => { refreshes++; });
  try {
    await b.tick();
    assert.equal(reads, 0);
    b.document.visibilityState = 'visible';
    b.document.dispatchEvent(new Event('visibilitychange'));
    await setImmediate();
    assert.equal(refreshes, 1);
  } finally { stop(); b.restore(); }
});

test('an in-flight result is discarded after logout or an account switch', async () => {
  const b = browser();
  markPaymentStarted('alice');
  let complete!: (value: { status: string; expiresAt: null }) => void;
  let refreshes = 0;
  const stop = watchPaymentAccess('alice', () => new Promise(resolve => { complete = resolve; }), async () => { refreshes++; });
  try {
    stop();
    complete({ status: 'active', expiresAt: null });
    await setImmediate();
    assert.equal(refreshes, 0);
  } finally { stop(); b.restore(); }
});
