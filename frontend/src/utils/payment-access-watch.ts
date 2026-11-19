const PENDING_KEY = 'astradia:pending-payment';
const STARTED_EVENT = 'astradia:payment-started';
const WATCH_MS = 60 * 60 * 1000;

type Membership = { status: string; expiresAt: string | null };

function pendingUntil(userId: string): number {
  try {
    const pending = JSON.parse(localStorage.getItem(PENDING_KEY) || 'null');
    return pending?.userId === userId && Number.isFinite(pending.until) ? pending.until : 0;
  } catch { return 0; }
}

export function clearPendingPayment(userId: string): void {
  if (!pendingUntil(userId)) return;
  try { localStorage.removeItem(PENDING_KEY); } catch { /* Storage can be unavailable. */ }
}

export function markPaymentStarted(userId: string): void {
  try {
    localStorage.setItem(PENDING_KEY, JSON.stringify({ userId, until: Date.now() + WATCH_MS }));
  } catch { /* The same-tab event still starts the watcher. */ }
  window.dispatchEvent(new CustomEvent(STARTED_EVENT, { detail: { userId } }));
}

// Payment notifications grant access on the server. This only refreshes the
// browser's authenticated membership, including after the checkout modal closes.
export function watchPaymentAccess(
  userId: string,
  readMembership: () => Promise<Membership>,
  refreshAuth: () => Promise<void>,
): () => void {
  let until = pendingUntil(userId);
  let stopped = false;
  let busy = false;
  if (new URLSearchParams(window.location.search).get('payment') === 'success') {
    markPaymentStarted(userId);
    until = Date.now() + WATCH_MS;
  }

  const check = async () => {
    if (stopped || busy || document.visibilityState === 'hidden') return;
    busy = true;
    try {
      const membership = await readMembership();
      if (stopped) return;
      const active = membership.status === 'active'
        && (!membership.expiresAt || Date.parse(membership.expiresAt) > Date.now());
      if (active) {
        await refreshAuth();
        if (stopped) return;
        clearPendingPayment(userId);
        stop();
      }
    } catch { /* A temporary network error must not sign the user out. */ }
    finally { busy = false; }
  };
  const onFocus = () => { void check(); };
  const onStart = (event: Event) => {
    if ((event as CustomEvent<{ userId: string }>).detail?.userId !== userId) return;
    until = Date.now() + WATCH_MS;
    void check();
  };
  const onStorage = (event: StorageEvent) => {
    if (event.key !== PENDING_KEY) return;
    until = pendingUntil(userId);
    if (until > Date.now()) void check();
  };
  const timer = window.setInterval(() => {
    if (until > Date.now()) void check();
  }, 5000);
  function stop() {
    stopped = true;
    window.clearInterval(timer);
    window.removeEventListener('focus', onFocus);
    window.removeEventListener(STARTED_EVENT, onStart);
    window.removeEventListener('storage', onStorage);
    document.removeEventListener('visibilitychange', onFocus);
  }
  window.addEventListener('focus', onFocus);
  window.addEventListener(STARTED_EVENT, onStart);
  window.addEventListener('storage', onStorage);
  document.addEventListener('visibilitychange', onFocus);
  if (until > Date.now()) void check();
  return stop;
}
