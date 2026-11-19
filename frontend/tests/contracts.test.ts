import test from 'node:test';
import assert from 'node:assert/strict';
import { parseSignalSummary, unavailableSignal } from '../src/contracts.ts';

test('the public client rejects internal evaluation fields', () => {
  assert.deepEqual(parseSignalSummary(unavailableSignal), unavailableSignal);
  for (const field of ['weights', 'coefficients', 'calibration', 'indicators']) {
    assert.throws(() => parseSignalSummary({ ...unavailableSignal, [field]: {} }));
  }
});
test('an unavailable result cannot become a local trading recommendation', () => {
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, action: 'LONG' }));
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, state: 'available' }));
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, asOf: 'invalid' }));
});
