import test from 'node:test';
import assert from 'node:assert/strict';
import { birthInstant } from '../src/utils/birthData.js';

const at = (timezoneOffset: number) => birthInstant({ date: '2000-01-01', time: '10:30', timezoneOffset }).toISOString();

test('a birth entered as local time becomes the right instant', () => {
  assert.equal(at(0), '2000-01-01T10:30:00.000Z');
  assert.equal(at(1), '2000-01-01T09:30:00.000Z');
  assert.equal(at(-5), '2000-01-01T15:30:00.000Z');
});

test('a half-hour or quarter-hour zone keeps its fraction', () => {
  // Handing Date.UTC an hour such as 4.5 drops the fraction: these were up to 45 minutes off.
  assert.equal(at(5.5), '2000-01-01T05:00:00.000Z');
  assert.equal(at(5.75), '2000-01-01T04:45:00.000Z');
  assert.equal(at(-3.5), '2000-01-01T14:00:00.000Z');
});

test('the offset can carry the instant across midnight', () => {
  assert.equal(birthInstant({ date: '2000-01-01', time: '00:15', timezoneOffset: 9.5 }).toISOString(), '1999-12-31T14:45:00.000Z');
  assert.equal(birthInstant({ date: '1999-12-31', time: '23:50', timezoneOffset: -8 }).toISOString(), '2000-01-01T07:50:00.000Z');
});

test('a date or time with a field missing gives an invalid Date', () => {
  assert.ok(Number.isNaN(birthInstant({ date: '2000-01', time: '10:30', timezoneOffset: 0 }).getTime()));
  assert.ok(Number.isNaN(birthInstant({ date: '2000-01-01', time: '', timezoneOffset: 0 }).getTime()));
});
