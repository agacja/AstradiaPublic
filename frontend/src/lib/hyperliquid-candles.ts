// Public market data only: Hyperliquid's unauthenticated candle snapshot.
// The product reads candles through an exchange SDK client that also carries
// account and order functions; this file is the read-only equivalent.
import { isList, isRecord } from './guards';

export interface Candle {
  /** When the candle opened, in milliseconds since the epoch. */
  t: number;
  o: string;
  h: string;
  l: string;
  c: string;
  v: string;
}

const INFO_URL = 'https://api.hyperliquid.xyz/info';
/** The exchange must answer or fail: a request that hangs would leave the chart loading for ever. */
export const REQUEST_TIMEOUT_MS = 15_000;
/** A price or a volume as the exchange writes it: a decimal number in text. */
const DECIMAL = /^\d+(?:\.\d+)?(?:[eE][+-]?\d+)?$/;

/** The fields the chart reads. A candle may carry others, which are passed through unread. */
function isCandle(value: unknown): value is Candle {
  if (!isRecord(value)) return false;
  const { t, o, h, l, c, v } = value;
  return typeof t === 'number' && Number.isFinite(t)
    && [o, h, l, c, v].every(field => typeof field === 'string' && DECIMAL.test(field));
}

/**
 * Reads a candle snapshot from an untrusted response body. Anything that is
 * not a list of candles is refused here, at the boundary, so the chart never
 * draws from a body of another shape or from a price that is not a number.
 */
export function parseCandles(body: unknown): Candle[] {
  if (!isList(body) || !body.every(isCandle)) throw new Error('Candle response was not a list of candles');
  return body;
}

export async function getCandles(coin: string, interval: string, startTime: number, endTime?: number): Promise<Candle[]> {
  const response = await fetch(INFO_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type: 'candleSnapshot', req: { coin, interval, startTime, endTime: endTime ?? Date.now() } }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) throw new Error(`Candle request failed: ${response.status}`);
  return parseCandles(await response.json());
}
