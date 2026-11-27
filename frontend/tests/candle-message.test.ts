import test from 'node:test';
import assert from 'node:assert/strict';
import { readLiveCandle } from '../src/utils/candle-message.js';

// A message of the candle channel as the exchange sends it.
const data = { t: 1_767_225_600_000, T: 1_767_226_499_999, s: 'BTC', i: '15m', o: '100.0', c: '101.5', h: '102.25', l: '99.5', v: '12.3', n: 42 };
const message = { channel: 'candle', data };

test('a candle of the stream is read as numbers, its time in seconds', () => {
  assert.deepEqual(readLiveCandle(message), { time: 1_767_225_600, open: 100, high: 102.25, low: 99.5, close: 101.5, volume: 12.3 });
  // Through JSON, as it arrives.
  assert.deepEqual(readLiveCandle(JSON.parse(JSON.stringify(message))), readLiveCandle(message));
  assert.equal(readLiveCandle({ channel: 'candle', data: { ...data, t: 1_767_225_600_999 } })?.time, 1_767_225_600);
});

test('any other message is not a candle', () => {
  for (const other of [null, undefined, 'pong', 7, [], { channel: 'pong' }, { channel: 'subscriptionResponse', data }, { channel: 'candle' }, { channel: 'candle', data: null }, { channel: 'candle', data: 'x' }, { data }]) {
    assert.equal(readLiveCandle(other), null, JSON.stringify(other));
  }
});

test('a candle with a field that is missing or not a number is dropped, so nothing is drawn from NaN', () => {
  const withoutVolume = Object.fromEntries(Object.entries(data).filter(([field]) => field !== 'v'));
  const wrong: unknown[] = [
    withoutVolume,
    { ...data, t: undefined }, { ...data, t: '1767225600000x' }, { ...data, t: null },
    { ...data, o: 'abc' }, { ...data, h: '' }, { ...data, l: ' ' }, { ...data, c: null }, { ...data, c: {} }, { ...data, v: '12abc' },
    { ...data, o: Number.NaN }, { ...data, c: Number.POSITIVE_INFINITY },
  ];
  for (const broken of wrong) assert.equal(readLiveCandle({ channel: 'candle', data: broken }), null, JSON.stringify(broken));
});
