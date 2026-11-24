import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reconnectDelay } from '../src/utils/reconnect-delay.ts';

test('a dropped stream is retried after 3s, then less and less often, at most every 30s', () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5, 50].map(reconnectDelay), [3000, 6000, 12000, 24000, 30000, 30000, 30000]);
});

test('a nonsense attempt count still gives a sane wait', () => {
  for (const bad of [-1, NaN, Infinity, 2.7]) {
    const wait = reconnectDelay(bad);
    assert.ok(wait >= 3000 && wait <= 30000, `${bad} -> ${wait}`);
  }
});
