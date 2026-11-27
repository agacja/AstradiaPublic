// One candle from the exchange's live stream, read without trusting the message.

/** A candle as numbers. `time` is when it opened, in seconds since the epoch. */
export interface LiveCandle {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null;

/** A number sent as a number or as decimal text. Anything else is NaN. */
const toNumber = (value: unknown): number =>
  typeof value === 'number' ? value : typeof value === 'string' && value.trim() !== '' ? Number(value) : NaN;

/**
 * The candle in a message of the candle channel, or null for any other
 * message and for a candle with a field that is missing or not a number: the
 * chart and the price are never drawn from NaN.
 */
export function readLiveCandle(message: unknown): LiveCandle | null {
  if (!isRecord(message)) return null;
  const { channel, data } = message;
  if (channel !== 'candle' || !isRecord(data)) return null;
  const { t, o, h, l, c, v } = data;
  const candle = {
    time: Math.floor(toNumber(t) / 1000),
    open: toNumber(o),
    high: toNumber(h),
    low: toNumber(l),
    close: toNumber(c),
    volume: toNumber(v),
  };
  return Object.values(candle).every(Number.isFinite) ? candle : null;
}
