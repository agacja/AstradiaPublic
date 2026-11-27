import assert from 'node:assert/strict';
import test from 'node:test';
import { REQUEST_TIMEOUT_MS, getCandles, parseCandles } from '../src/lib/hyperliquid-candles.ts';

const candle = { t: 1_767_225_600_000, T: 1_767_226_499_999, s: 'BTC', i: '15m', o: '100.0', c: '101.5', h: '102.0', l: '99.5', v: '12.3', n: 42 };

/** Replaces `fetch` for one test, and puts the real one back afterwards. */
async function withFetch(replacement: typeof fetch, run: () => Promise<void>): Promise<void> {
  const real = globalThis.fetch;
  globalThis.fetch = replacement;
  try {
    await run();
  } finally {
    globalThis.fetch = real;
  }
}

test('a list of candles is read as it is, an empty one too', () => {
  assert.deepEqual(parseCandles([candle, candle]), [candle, candle]);
  assert.deepEqual(parseCandles([]), []);
});

test('a body of any other shape is refused before the chart sees it', () => {
  const { v: _dropped, ...withoutVolume } = candle;
  const wrong: unknown[] = [
    null, undefined, 'candles', 7, {}, { data: [candle] },
    [null], [[candle]], [withoutVolume],
    [{ ...candle, t: '1767225600000' }], [{ ...candle, t: Number.NaN }], [{ ...candle, o: 100 }],
    // A price must be a number in text: the chart would draw anything else as NaN.
    [{ ...candle, o: 'abc' }], [{ ...candle, h: '' }], [{ ...candle, v: '1,5' }], [{ ...candle, l: '-1' }],
    // One bad candle refuses the whole list: half a chart is not drawn.
    [candle, { ...candle, c: null }],
  ];
  for (const body of wrong) assert.throws(() => parseCandles(body), JSON.stringify(body));
});

test('the request is the public snapshot and carries no credential', async () => {
  let asked: { url: string; init: RequestInit | undefined } | undefined;
  await withFetch(
    async (url, init) => {
      asked = { url: String(url), init };
      return Response.json([candle]);
    },
    async () => {
      assert.deepEqual(await getCandles('BTC', '15m', 1_767_000_000_000, 1_767_225_600_000), [candle]);
    },
  );
  assert.ok(asked);
  assert.equal(asked.url, 'https://api.hyperliquid.xyz/info');
  assert.equal(asked.init?.method, 'POST');
  assert.deepEqual(JSON.parse(String(asked.init?.body)), {
    type: 'candleSnapshot',
    req: { coin: 'BTC', interval: '15m', startTime: 1_767_000_000_000, endTime: 1_767_225_600_000 },
  });
  assert.deepEqual(asked.init?.headers, { 'Content-Type': 'application/json' });
  assert.equal(asked.init?.credentials, undefined);
});

test('a request that is not answered in time fails the call', async t => {
  // The limit the module asks for, answered with a signal whose time is already up.
  const limits: number[] = [];
  t.mock.method(AbortSignal, 'timeout', (milliseconds: number) => {
    limits.push(milliseconds);
    return AbortSignal.abort(new DOMException('The operation timed out.', 'TimeoutError'));
  });
  await withFetch(
    async (_url, init) => {
      init?.signal?.throwIfAborted();
      return Response.json([candle]);
    },
    async () => {
      await assert.rejects(getCandles('BTC', '15m', 0), { name: 'TimeoutError' });
    },
  );
  assert.deepEqual(limits, [REQUEST_TIMEOUT_MS]);
  assert.equal(REQUEST_TIMEOUT_MS, 15_000);
});

test('a refused request and a body that is not candles both fail the call', async () => {
  await withFetch(
    async () => new Response('rate limited', { status: 429 }),
    async () => {
      await assert.rejects(getCandles('BTC', '15m', 0), { message: 'Candle request failed: 429' });
    },
  );
  await withFetch(
    async () => Response.json({ error: 'unknown coin' }),
    async () => {
      await assert.rejects(getCandles('NOPE', '15m', 0), { message: 'Candle response was not a list of candles' });
    },
  );
});
