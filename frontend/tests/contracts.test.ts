import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  MODEL_VERSION_MAX_LENGTH, SIGNAL_ACTIONS, SIGNAL_FIELDS, SIGNAL_STATES, SYMBOL_PATTERN, parseSignalSummary, unavailableSignal,
} from '../src/contracts.ts';
import type { SignalSummary } from '../src/contracts.ts';

const available: SignalSummary = {
  symbol: 'DEMO',
  asOf: '2026-01-01T00:00:00Z',
  action: 'LONG',
  state: 'available',
  modelVersion: 'example-1',
};

test('a valid summary comes back as it was sent, in either state', () => {
  assert.deepEqual(parseSignalSummary(unavailableSignal), unavailableSignal);
  assert.deepEqual(parseSignalSummary(available), available);
  for (const asOf of ['2026-01-01T00:00:00.250+02:00', '2024-02-29T23:59:59Z', '2026-12-31T00:00:00-23:59']) {
    assert.equal(parseSignalSummary({ ...available, asOf }).asOf, asOf);
  }
  // What is returned is a new object: nothing of the input is retained.
  assert.notEqual(parseSignalSummary(available), available);
});

test('the public client rejects internal evaluation fields', () => {
  for (const field of ['weights', 'coefficients', 'calibration', 'indicators']) {
    assert.throws(() => parseSignalSummary({ ...unavailableSignal, [field]: {} }));
    assert.throws(() => parseSignalSummary({ ...available, [field]: {} }));
  }
});

test('an unavailable result cannot become a local trading recommendation', () => {
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, action: 'LONG' }));
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, state: 'available' }));
  assert.throws(() => parseSignalSummary({ ...unavailableSignal, asOf: 'invalid' }));
});

test('an available result says when it was computed and by which model version', () => {
  assert.throws(() => parseSignalSummary({ ...available, asOf: null }));
  assert.throws(() => parseSignalSummary({ ...available, modelVersion: null }));
  assert.throws(() => parseSignalSummary({ ...available, modelVersion: '' }));
  assert.throws(() => parseSignalSummary({ ...available, modelVersion: 'x'.repeat(81) }));
});

test('a field of the wrong shape is refused', () => {
  const wrong: [string, unknown][] = [
    ['symbol', 'demo'], ['symbol', ''], ['symbol', 'A'.repeat(21)], ['symbol', 7],
    ['action', 'BUY'], ['action', null], ['action', ['LONG']],
    ['state', 'pending'], ['state', true],
    // A date-time, not anything a date parser accepts.
    ['asOf', '2026'], ['asOf', 'January 1, 2026'], ['asOf', '2026-01-01'], ['asOf', 1767225600], ['asOf', '2026-01-01t00:00:00z'],
    // A date or a time that does not exist, which a date parser may move to the next day or month.
    ['asOf', '2026-13-01T00:00:00Z'], ['asOf', '2026-02-29T00:00:00Z'], ['asOf', '2026-02-30T00:00:00Z'], ['asOf', '2026-04-31T00:00:00Z'],
    ['asOf', '2026-01-00T00:00:00Z'], ['asOf', '2026-01-01T24:00:00Z'], ['asOf', '2026-01-01T00:60:00Z'], ['asOf', '2026-01-01T00:00:60Z'],
    ['asOf', '2026-01-01T00:00:00+24:00'],
    ['modelVersion', 3],
  ];
  for (const [field, value] of wrong) {
    assert.throws(() => parseSignalSummary({ ...available, [field]: value }), `${field}: ${JSON.stringify(value)}`);
  }
});

test('what is not an object with exactly the five fields is refused', () => {
  for (const value of [null, undefined, 'DEMO', 7, [], [available], {}]) {
    assert.throws(() => parseSignalSummary(value), JSON.stringify(value));
  }
  const { modelVersion: _dropped, ...missing } = available;
  assert.throws(() => parseSignalSummary(missing));
  // A field that is only inherited is not a field of the response.
  assert.throws(() => parseSignalSummary(Object.create(available)));
  assert.throws(() => parseSignalSummary({ ...missing, extra: 1 }));
  // Five own fields, one of them hidden and an extra one in its place: the extra one is not let through.
  const hidden = Object.defineProperty({ ...missing, weights: {} }, 'modelVersion', { value: 'example-1', enumerable: false });
  assert.throws(() => parseSignalSummary(hidden));
});

test('the parser and the published JSON schema describe the same contract', () => {
  const schema: unknown = JSON.parse(readFileSync(new URL('../../contracts/signal-summary.schema.json', import.meta.url), 'utf8'));
  /** The value at a path of the schema, or undefined where the path does not lead. */
  const at = (...path: string[]) =>
    path.reduce<unknown>((node, key) => (typeof node === 'object' && node !== null ? Reflect.get(node, key) : undefined), schema);
  assert.equal(at('additionalProperties'), false);
  assert.deepEqual(at('required'), [...SIGNAL_FIELDS]);
  assert.deepEqual(Object.keys(Object(at('properties'))), [...SIGNAL_FIELDS]);
  assert.deepEqual(at('properties', 'action', 'enum'), [...SIGNAL_ACTIONS]);
  assert.deepEqual(at('properties', 'state', 'enum'), [...SIGNAL_STATES]);
  assert.equal(at('properties', 'symbol', 'pattern'), SYMBOL_PATTERN.source);
  assert.equal(at('properties', 'modelVersion', 'maxLength'), MODEL_VERSION_MAX_LENGTH);
});

test('every refusal has the same message, which says nothing about the response', () => {
  for (const value of [null, { ...available, weights: { secret: 1 } }, { ...available, symbol: 'not a symbol' }]) {
    assert.throws(() => parseSignalSummary(value), { message: 'Invalid signal response' });
  }
});
