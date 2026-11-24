import test from 'node:test';
import assert from 'node:assert/strict';
import { setImmediate } from 'node:timers/promises';
import { clearPendingPayment, markPaymentStarted, paymentLanded, renewalPending, watchPaymentAccess } from '../src/utils/payment-access-watch.ts';

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

test('a refresh that fails right after access is granted keeps the checkout pending and is tried again', async () => {
  const b = browser();
  markPaymentStarted('alice');
  let refreshes = 0;
  // The membership is active, but the first refresh cannot reach the server.
  // It rejects (the member stays signed in); the watcher must not treat the
  // payment as handled.
  const stop = watchPaymentAccess('alice', async () => ({ status: 'active', expiresAt: null }), async () => {
    if (++refreshes === 1) throw new TypeError('Failed to fetch');
  });
  try {
    await setImmediate();
    assert.equal(refreshes, 1);
    assert.equal(b.storage.size, 1, 'the checkout is still pending');
    assert.equal(b.timers.size, 1, 'the watcher is still running');
    await b.tick();
    assert.equal(refreshes, 2);
    assert.equal(b.storage.size, 0);
    assert.equal(b.timers.size, 0);
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

test('a member paying for another month is watched until the month has been added', async () => {
  const b = browser();
  const day = 86_400_000;
  const endsNow = Date.now() + 2 * day;
  let expiresAt = new Date(endsNow).toISOString(), reads = 0, refreshes = 0;
  const stop = watchPaymentAccess('alice', async () => { reads++; return { status: 'active', expiresAt }; }, async () => { refreshes++; }, true);
  try {
    await b.tick();
    assert.equal(reads, 0, 'a paid-up member with no payment started is never polled');
    // Active all along: being active is not the payment arriving.
    markPaymentStarted('alice', endsNow);
    await setImmediate();
    assert.equal(reads, 1);
    assert.equal(refreshes, 0);
    await b.tick();
    assert.equal(refreshes, 0);
    assert.equal(b.storage.size, 1, 'the payment is still being waited for');
    // The month is added to the time that was left.
    expiresAt = new Date(endsNow + 30 * day).toISOString();
    await b.tick();
    assert.equal(refreshes, 1);
    assert.equal(b.storage.size, 0);
    assert.equal(b.timers.size, 0);
  } finally { stop(); b.restore(); }
});

test('a renewal being waited for survives a reload with the end date it started from', async () => {
  const b = browser();
  const day = 86_400_000;
  const endsNow = Date.now() + day;
  let expiresAt = new Date(endsNow).toISOString(), refreshes = 0;
  markPaymentStarted('alice', endsNow);
  // The page is loaded again: a new watcher reads what was stored.
  const stop = watchPaymentAccess('alice', async () => ({ status: 'active', expiresAt }), async () => { refreshes++; });
  try {
    await setImmediate();
    assert.equal(refreshes, 0);
    expiresAt = new Date(endsNow + 30 * day).toISOString();
    await b.tick();
    assert.equal(refreshes, 1);
  } finally { stop(); b.restore(); }
});

test('a payment has arrived when access begins, or when a running membership ends later than before', () => {
  const end = 1_800_000_000_000;
  // A member who was not paid up: active is the whole answer.
  assert.equal(paymentLanded(true, null, null), true);
  assert.equal(paymentLanded(true, end, null), true);
  assert.equal(paymentLanded(false, null, null), false);
  // A member who was paid up until `end`: only a later end counts.
  assert.equal(paymentLanded(true, end, end), false);
  assert.equal(paymentLanded(true, end - 1, end), false);
  assert.equal(paymentLanded(true, end + 1, end), true);
  assert.equal(paymentLanded(true, null, end), false);
  assert.equal(paymentLanded(false, end + 1, end), false);
});

test('a paid-up member coming back to the page is not looked up, and their account is not reloaded', async () => {
  const b = browser();
  let reads = 0, refreshes = 0;
  const stop = watchPaymentAccess('alice', async () => { reads++; return { status: 'active', expiresAt: null }; }, async () => { refreshes++; }, true);
  try {
    // Returning to the tab, again and again, with no payment being waited for.
    for (let n = 0; n < 5; n++) {
      b.window.dispatchEvent(new Event('focus'));
      b.document.dispatchEvent(new Event('visibilitychange'));
      await b.tick();
    }
    assert.equal(reads, 0);
    assert.equal(refreshes, 0);
    // Once they start paying for another month, coming back does look them up.
    const endsNow = Date.now() + 86_400_000;
    markPaymentStarted('alice', endsNow);
    await setImmediate();
    assert.equal(renewalPending('alice'), true);
    const before = reads;
    b.window.dispatchEvent(new Event('focus'));
    await setImmediate();
    assert.ok(reads > before);
    assert.equal(refreshes, 0, 'active all along is not the month arriving');
  } finally { stop(); b.restore(); }
});

test('a member without access is still looked up each time they come back to the page', async () => {
  const b = browser();
  let reads = 0;
  const stop = watchPaymentAccess('alice', async () => { reads++; return { status: 'none', expiresAt: null }; }, async () => {});
  try {
    b.window.dispatchEvent(new Event('focus'));
    await setImmediate();
    assert.equal(reads, 1);
    assert.equal(renewalPending('alice'), false);
  } finally { stop(); b.restore(); }
});
