/**
 * How long a live chart waits before reconnecting a stream that dropped:
 * three seconds at first, doubling with each failed attempt in a row, and
 * never more than thirty. A source that is down is then asked a couple of
 * times a minute instead of every three seconds for as long as the page is
 * open. The count starts again once a connection opens.
 */
export function reconnectDelay(failedAttempts: number): number {
  const attempts = Number.isFinite(failedAttempts) ? Math.max(0, Math.floor(failedAttempts)) : 0;
  return Math.min(3000 * 2 ** Math.min(attempts, 10), 30000);
}
