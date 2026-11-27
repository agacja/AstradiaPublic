import { API_BASE } from './session';
import { failure, readBriefing, type DailyBriefing } from './briefing-response';

export type { DailyBriefing };

export async function loadDailyBriefing(token: string, generate: boolean, signal: AbortSignal): Promise<DailyBriefing> {
  const response = await fetch(`${API_BASE}/briefings/daily`, {
    method: generate ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store', signal,
  });
  return readBriefing(response);
}

export async function loadBriefingAudio(token: string, id: string, signal: AbortSignal): Promise<Blob> {
  const response = await fetch(`${API_BASE}/briefings/daily/audio?id=${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` }, cache: 'no-store', signal,
  });
  if (!response.ok) throw await failure(response, 'The audio could not be loaded. Please try again.');
  if (!response.headers.get('content-type')?.startsWith('audio/')) throw new Error('The audio could not be loaded.');
  return response.blob();
}
