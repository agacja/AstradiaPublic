import { API_BASE } from './session';

export interface DailyBriefing {
  id: string;
  date: string;
  transcript: string;
  estimatedSeconds: number;
  status: 'ready' | 'generating' | 'available' | 'unavailable' | 'limit_reached';
  canGenerate: boolean;
}

export async function loadDailyBriefing(token: string, generate: boolean, signal: AbortSignal): Promise<DailyBriefing> {
  const response = await fetch(`${API_BASE}/briefings/daily`, {
    method: generate ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${token}` },
    cache: 'no-store', signal,
  });
  const data = await response.json();
  if (!response.ok || !data.success || !data.briefing) throw new Error(data.error || 'Your briefing could not be loaded.');
  return data.briefing;
}

export async function loadBriefingAudio(token: string, id: string, signal: AbortSignal): Promise<Blob> {
  const response = await fetch(`${API_BASE}/briefings/daily/audio?id=${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` }, cache: 'no-store', signal,
  });
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(data.error || 'The audio could not be loaded. Please try again.');
  }
  if (!response.headers.get('content-type')?.startsWith('audio/')) throw new Error('The audio could not be loaded.');
  return response.blob();
}
