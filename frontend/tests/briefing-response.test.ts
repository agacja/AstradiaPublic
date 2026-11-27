import test from 'node:test';
import assert from 'node:assert/strict';
import { failure, readBriefing } from '../src/utils/briefing-response.js';

const briefing = {
  id: 'a'.repeat(64),
  date: '2026-01-01',
  transcript: 'Good morning.',
  estimatedSeconds: 42,
  status: 'available',
  canGenerate: true,
};
const json = (body: unknown, status = 200) => Response.json(body, { status });
const OURS = 'Your briefing could not be loaded.';

test('a briefing the server sent comes back as it is, in every status', async () => {
  for (const status of ['ready', 'generating', 'available', 'unavailable', 'limit_reached']) {
    const sent = { ...briefing, status, canGenerate: status === 'available' };
    assert.deepEqual(await readBriefing(json({ success: true, briefing: sent })), sent);
  }
});

test('an answer that is not JSON gets the message a member can read, never a parser error', async () => {
  // What a proxy answers while the server is away.
  const page = new Response('<!DOCTYPE html><title>502 Bad Gateway</title>', { status: 502, headers: { 'content-type': 'text/html' } });
  await assert.rejects(readBriefing(page), { message: OURS });
  await assert.rejects(readBriefing(new Response('', { status: 504 })), { message: OURS });
  await assert.rejects(readBriefing(new Response('<html>', { status: 200 })), { message: OURS });
  for (const body of [null, 7, 'text', [briefing]]) await assert.rejects(readBriefing(json(body)), { message: OURS });
});

test('a failure is told in the server\'s own words when it sent any', async () => {
  await assert.rejects(readBriefing(json({ success: false, error: 'Membership required' }, 403)), { message: 'Membership required' });
  await assert.rejects(readBriefing(json({ success: false }, 500)), { message: OURS });
  // Words only: nothing else the server put under `error` reaches the page.
  for (const error of ['', 7, null, { detail: 'x' }, ['x']]) {
    await assert.rejects(readBriefing(json({ success: false, error }, 500)), { message: OURS });
  }
  assert.equal((await failure(json({ error: 'Too many requests' }, 429), 'Audio failed.')).message, 'Too many requests');
  assert.equal((await failure(new Response('<html>', { status: 502 }), 'Audio failed.')).message, 'Audio failed.');
});

test('a briefing is whole or it is not a briefing', async () => {
  // A success needs both the flag and the status line.
  await assert.rejects(readBriefing(json({ success: true, briefing }, 500)), { message: OURS });
  await assert.rejects(readBriefing(json({ success: false, briefing })), { message: OURS });
  await assert.rejects(readBriefing(json({ success: true })), { message: OURS });
  const wrong: [string, unknown][] = [
    ['id', 7], ['date', null], ['transcript', undefined], ['estimatedSeconds', '42'],
    ['status', 'done'], ['status', undefined], ['canGenerate', 'yes'],
  ];
  for (const [field, value] of wrong) {
    await assert.rejects(readBriefing(json({ success: true, briefing: { ...briefing, [field]: value } })), { message: OURS }, `${field}: ${String(value)}`);
  }
});
