// What the briefing routes answer, read without trusting it. This module has
// no import of its own, so its checks can be tested outside the browser.

const STATUSES = ['ready', 'generating', 'available', 'unavailable', 'limit_reached'] as const;

export interface DailyBriefing {
  id: string;
  date: string;
  transcript: string;
  estimatedSeconds: number;
  status: (typeof STATUSES)[number];
  canGenerate: boolean;
}

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null;

/** Whether this is a whole briefing: the player reads every one of these fields. */
function isBriefing(value: unknown): value is DailyBriefing {
  if (!isRecord(value)) return false;
  const { id, date, transcript, estimatedSeconds, status, canGenerate } = value;
  return typeof id === 'string' && typeof date === 'string' && typeof transcript === 'string'
    && typeof estimatedSeconds === 'number' && typeof canGenerate === 'boolean'
    && STATUSES.some(known => known === status);
}

/** The body of an answer when it is JSON. An error page from a proxy is not, and reads as null. */
const body = (response: Response): Promise<unknown> => response.json().catch(() => null);

/** The server's own words for a failure when it sent any, ours otherwise. */
function message(data: unknown, ours: string): string {
  const theirs = isRecord(data) ? data['error'] : undefined;
  return typeof theirs === 'string' && theirs ? theirs : ours;
}

/** What went wrong with an answer that is not a success, as an error a member can read. */
export async function failure(response: Response, ours: string): Promise<Error> {
  return new Error(message(await body(response), ours));
}

/** The briefing in a successful answer. Anything else is an error a member can read. */
export async function readBriefing(response: Response): Promise<DailyBriefing> {
  const data = await body(response);
  const briefing = response.ok && isRecord(data) && data['success'] ? data['briefing'] : null;
  if (!isBriefing(briefing)) throw new Error(message(data, 'Your briefing could not be loaded.'));
  return briefing;
}
