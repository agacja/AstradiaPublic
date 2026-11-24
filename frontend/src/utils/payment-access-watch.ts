const PENDING_KEY = 'astradia:pending-payment';
const STARTED_EVENT = 'astradia:payment-started';
const WATCH_MS = 60 * 60 * 1000;

type Membership = { status: string; expiresAt: string | null };
/** A payment being waited for: how long to keep looking, and, for a member
 * who was already paid up when it started, the end of the membership then. */
type Pending = { until: number; paidUntil: number | null };

function pending(userId: string): Pending {
  try {
    const stored = JSON.parse(localStorage.getItem(PENDING_KEY) || 'null');
    if (stored?.userId === userId && Number.isFinite(stored.until)) {
      return { until: stored.until, paidUntil: Number.isFinite(stored.paidUntil) ? stored.paidUntil : null };
    }
  } catch { /* Storage can be unavailable. */ }
  return { until: 0, paidUntil: null };
}

export function clearPendingPayment(userId: string): void {
  if (!pending(userId).until) return;
  try { localStorage.removeItem(PENDING_KEY); } catch { /* Storage can be unavailable. */ }
}

/**
 * Whether the payment being waited for has arrived on the membership.
 *
 * A new member's payment has arrived once the membership is active. A member
 * who pays for another month is active all along, so that says nothing: their
 * payment has arrived once the membership ends later than it did when the
 * payment started (`paidUntilAtStart`, null for a member who was not paid up).
 */
export function paymentLanded(active: boolean, paidUntil: number | null, paidUntilAtStart: number | null): boolean {
  if (!active) return false;
  return paidUntilAtStart === null || (paidUntil !== null && paidUntil > paidUntilAtStart);
}

/** Whether this member started paying for another month and it has not arrived yet. */
export function renewalPending(userId: string): boolean {
  const { until, paidUntil } = pending(userId);
  return until > Date.now() && paidUntil !== null;
}

/** `paidUntil`: when the membership ends now, for a member paying for another
 * month; leave it out for a member who is not paid up. */
export function markPaymentStarted(userId: string, paidUntil: number | null = null): void {
  try {
    localStorage.setItem(PENDING_KEY, JSON.stringify({ userId, until: Date.now() + WATCH_MS, paidUntil }));
  } catch { /* The same-tab event still starts the watcher. */ }
  window.dispatchEvent(new CustomEvent(STARTED_EVENT, { detail: { userId, paidUntil } }));
}

/** The checkout's return address says a payment was just made. It says so
 * once: left in the address bar, it would start the watch again on every
 * later start of the watcher. */
function forgetSuccessAddress(): void {
  try {
    const url = new URL(window.location.href);
    url.searchParams.delete('payment');
    window.history.replaceState(window.history.state, '', url);
  } catch { /* No address bar to tidy. */ }
}

// Payment notifications grant access on the server. This only refreshes the
// browser's authenticated membership, including after the checkout modal closes.
//
// `paidUp`: the member has access already. They are looked up only while a
// payment they started is being waited for. Anyone else is also looked up
// each time they come back to the page, because their access may have been
// granted meanwhile. Looking up a paid-up member on every return to the page
// reloaded their account each time, and with it every screen that shows it.
export function watchPaymentAccess(
  userId: string,
  readMembership: () => Promise<Membership>,
  refreshAuth: () => Promise<void>,
  paidUp = false,
): () => void {
  let { until, paidUntil: paidUntilAtStart } = pending(userId);
  let stopped = false;
  let busy = false;
  if (new URLSearchParams(window.location.search).get('payment') === 'success') {
    markPaymentStarted(userId, paidUntilAtStart);
    until = Date.now() + WATCH_MS;
    forgetSuccessAddress();
  }

  const check = async () => {
    if (stopped || busy || document.visibilityState === 'hidden') return;
    busy = true;
    try {
      const membership = await readMembership();
      if (stopped) return;
      const paidUntil = membership.expiresAt ? Date.parse(membership.expiresAt) : null;
      const active = membership.status === 'active' && (paidUntil === null || paidUntil > Date.now());
      if (paymentLanded(active, paidUntil, paidUntilAtStart)) {
        await refreshAuth();
        if (stopped) return;
        clearPendingPayment(userId);
        stop();
      }
    } catch { /* A temporary network error must not sign the user out. */ }
    finally { busy = false; }
  };
  const onFocus = () => { if (!paidUp || until > Date.now()) void check(); };
  const onStart = (event: Event) => {
    const detail = (event as CustomEvent<{ userId: string; paidUntil?: number | null }>).detail;
    if (detail?.userId !== userId) return;
    until = Date.now() + WATCH_MS;
    paidUntilAtStart = detail.paidUntil ?? null;
    void check();
  };
  const onStorage = (event: StorageEvent) => {
    if (event.key !== PENDING_KEY) return;
    ({ until, paidUntil: paidUntilAtStart } = pending(userId));
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
