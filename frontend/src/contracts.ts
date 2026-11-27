/** Illustrative public integration contract, not an internal engine struct. */
import { isRecord } from './lib/guards';

/** Every field of the contract. A response with any other field is refused. */
export const SIGNAL_FIELDS = ['symbol', 'asOf', 'action', 'state', 'modelVersion'] as const;
export const SIGNAL_ACTIONS = ['WAIT', 'LONG', 'SHORT'] as const;
export const SIGNAL_STATES = ['available', 'unavailable'] as const;
export const SYMBOL_PATTERN = /^[A-Z0-9-]{1,20}$/;
export const MODEL_VERSION_MAX_LENGTH = 80;

export type SignalAction = (typeof SIGNAL_ACTIONS)[number];

/** No result. There is nothing to act on, so the only action it can carry is to wait. */
export interface UnavailableSignal {
  readonly symbol: string;
  readonly asOf: string | null;
  readonly action: 'WAIT';
  readonly state: 'unavailable';
  readonly modelVersion: string | null;
}

/** A result. It always says when it was computed and by which model version. */
export interface AvailableSignal {
  readonly symbol: string;
  readonly asOf: string;
  readonly action: SignalAction;
  readonly state: 'available';
  readonly modelVersion: string;
}

export type SignalSummary = UnavailableSignal | AvailableSignal;

export const unavailableSignal: UnavailableSignal = Object.freeze({
  symbol: 'DEMO',
  asOf: null,
  action: 'WAIT',
  state: 'unavailable',
  modelVersion: null,
});

// A date-time as RFC 3339 writes it, with two limits of this parser's own: the
// T and the Z are upper case, and a leap second (a second of 60) is refused.
const DATE_TIME = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/;

function daysInMonth(year: number, month: number): number {
  if (month === 2) return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0) ? 29 : 28;
  return [4, 6, 9, 11].includes(month) ? 30 : 31;
}

/**
 * Whether the text is a date-time that exists. The calendar is checked here
 * and not left to `Date.parse`, which engines implement differently and which
 * moves a day that does not exist, such as 30 February, into the next month.
 */
function isDateTime(value: unknown): value is string {
  if (typeof value !== 'string') return false;
  const fields = DATE_TIME.exec(value)?.slice(1).map(field => (field === undefined ? 0 : Number(field)));
  if (!fields) return false;
  const [year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0, offsetHour = 0, offsetMinute = 0] = fields;
  return month >= 1 && month <= 12 && day >= 1 && day <= daysInMonth(year, month)
    && hour <= 23 && minute <= 59 && second <= 59 && offsetHour <= 23 && offsetMinute <= 59;
}

const isAction = (value: unknown): value is SignalAction => SIGNAL_ACTIONS.some(action => action === value);
const isModelVersion = (value: unknown): value is string =>
  typeof value === 'string' && value.length <= MODEL_VERSION_MAX_LENGTH;

/** One message for every refusal: the response's content is never echoed. */
function invalid(): never {
  throw new Error('Invalid signal response');
}

/**
 * Reads a signal summary from untrusted input. It refuses extra fields instead
 * of rendering or retaining private evaluation data, and the result is built
 * from checked values only, never asserted from the input.
 */
export function parseSignalSummary(value: unknown): SignalSummary {
  if (!isRecord(value)) return invalid();
  // Exactly the contract's fields, each one the object's own and enumerable.
  const keys = Object.keys(value);
  if (keys.length !== SIGNAL_FIELDS.length || !SIGNAL_FIELDS.every(field => keys.includes(field))) return invalid();
  const { symbol, asOf, action, state, modelVersion } = value;
  if (typeof symbol !== 'string' || !SYMBOL_PATTERN.test(symbol)) return invalid();
  if (!isAction(action)) return invalid();
  if (asOf !== null && !isDateTime(asOf)) return invalid();
  if (modelVersion !== null && !isModelVersion(modelVersion)) return invalid();
  switch (state) {
    case 'unavailable':
      // An unavailable result cannot become a recommendation.
      if (action !== 'WAIT') return invalid();
      return { symbol, asOf, action, state, modelVersion };
    case 'available':
      if (asOf === null || modelVersion === null || modelVersion === '') return invalid();
      return { symbol, asOf, action, state, modelVersion };
    default:
      return invalid();
  }
}
