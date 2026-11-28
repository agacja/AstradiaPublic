import { useEffect, useRef, useState } from 'react';
import { Headphones, Loader, Play, RefreshCw } from 'lucide-react';
import { getAuthToken } from '../utils/session';
import { loadDailyBriefing, loadBriefingAudio, type DailyBriefing } from '../utils/audio-briefing-api';
import '../styles/audio-briefing.css';

/** Remount on account/profile/day changes so a previous user's audio cannot linger. */
export function DailyAudioBriefing({ profileKey = '' }: { profileKey?: string }) {
  const [token, setToken] = useState(getAuthToken);
  const [day, setDay] = useState(() => new Date().toISOString().slice(0, 10));
  useEffect(() => {
    const sync = () => { setToken(getAuthToken()); setDay(new Date().toISOString().slice(0, 10)); };
    window.addEventListener('auth-token-changed', sync);
    window.addEventListener('storage', sync);
    document.addEventListener('visibilitychange', sync);
    const timer = window.setInterval(sync, 60_000);
    return () => {
      window.removeEventListener('auth-token-changed', sync);
      window.removeEventListener('storage', sync);
      document.removeEventListener('visibilitychange', sync);
      window.clearInterval(timer);
    };
  }, []);
  if (!token) return null;
  return <BriefingPlayer key={`${token}:${profileKey}:${day}`} token={token} />;
}

function BriefingPlayer({ token }: { token: string }) {
  const [briefing, setBriefing] = useState<DailyBriefing | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [audioUrl, setAudioUrl] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement>(null);
  const actionRef = useRef<AbortController | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 20_000);
    setLoading(true);
    setError(null);
    loadDailyBriefing(token, false, controller.signal)
      .then(value => { if (!controller.signal.aborted) setBriefing(value); })
      .catch(e => { if (!controller.signal.aborted) setError(e.message); })
      .finally(() => { clearTimeout(timeout); if (!controller.signal.aborted) setLoading(false); });
    const timedOut = () => { setLoading(false); setError('Your briefing took too long to load. Please try again.'); };
    controller.signal.addEventListener('abort', timedOut);
    return () => { controller.signal.removeEventListener('abort', timedOut); clearTimeout(timeout); controller.abort(); };
  }, [token, reload]);

  useEffect(() => () => {
    const action = actionRef.current;
    actionRef.current = null;
    action?.abort();
  }, []);
  useEffect(() => {
    if (!audioUrl) return;
    // Some mobile browsers require another tap after asynchronous generation.
    // Native controls remain available when autoplay is blocked.
    void audioRef.current?.play().catch(() => {});
    return () => { URL.revokeObjectURL(audioUrl); };
  }, [audioUrl]);

  useEffect(() => {
    if (briefing?.status !== 'generating' || busy) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      loadDailyBriefing(token, false, controller.signal)
        .then(setBriefing)
        .catch(e => { if (!controller.signal.aborted) setError(e.message); });
    }, 3000);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [briefing, busy, token]);

  const listen = async () => {
    if (!briefing || busy) return;
    const controller = new AbortController();
    actionRef.current = controller;
    const timeout = setTimeout(() => controller.abort(), 65_000);
    setBusy(true);
    setError(null);
    try {
      const next = briefing.status === 'ready' ? briefing : await loadDailyBriefing(token, true, controller.signal);
      if (controller.signal.aborted) return;
      setBriefing(next);
      if (next.status === 'ready') {
        const blob = await loadBriefingAudio(token, next.id, controller.signal);
        if (!controller.signal.aborted) setAudioUrl(URL.createObjectURL(blob));
      }
    } catch (e) {
      if (!controller.signal.aborted) {
        setError(e instanceof Error ? e.message : 'Audio could not be loaded.');
        // Refresh budget/lease state after provider errors without making another paid request.
        try { setBriefing(await loadDailyBriefing(token, false, controller.signal)); } catch { /* retain transcript */ }
      }
    } finally {
      clearTimeout(timeout);
      if (actionRef.current === controller) {
        setBusy(false);
        if (controller.signal.aborted) setError('Audio is taking longer than expected. Reload your briefing to check its status.');
      }
    }
  };

  const generating = busy || briefing?.status === 'generating';
  const date = briefing && new Date(`${briefing.date}T12:00:00Z`).toLocaleDateString('en-US', {
    weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC',
  });
  return (
    <section className="astr-card ab-card" aria-label="Your daily audio briefing">
      <div className="ab-heading">
        <span className="ab-icon" aria-hidden="true"><Headphones size={23} /></span>
        <div className="ab-title">
          <p className="ab-eyebrow">A moment for your day</p>
          <h2 className="astr-card-title">Your daily briefing</h2>
          <p className="astr-card-desc">Your personal reading, ready to listen to.</p>
        </div>
        <div className="ab-wave" aria-hidden="true">{[12, 23, 16, 33, 23, 42, 28, 16, 30, 20, 12].map((height, i) => <span key={i} style={{ height }} />)}</div>
      </div>
      <div className="ab-body">
        <div className="ab-meta">{date ? `${date} · UTC · About ${Math.max(1, Math.round(briefing!.estimatedSeconds / 60))} min` : 'A short daily reading from your natal chart'}</div>
        <div role="status" aria-live="polite">
          {loading && <p className="ab-status"><Loader size={15} className="ab-spin" /> Preparing your reading…</p>}
          {generating && <p className="ab-status"><Loader size={15} className="ab-spin" /> Preparing your audio. This may take a moment…</p>}
          {briefing?.status === 'unavailable' && <p className="ab-status">Audio is not available yet. Your written briefing is below.</p>}
          {briefing?.status === 'limit_reached' && <p className="ab-status">Today’s audio generation limit has been reached. Your written briefing is still available.</p>}
        </div>
        {error && <div className="ab-error" role="alert"><p>{error}</p><button type="button" className="astr-btn-ghost" disabled={busy} onClick={() => setReload(n => n + 1)}><RefreshCw size={14} /> Reload briefing</button></div>}
        {briefing && !audioUrl && (briefing.canGenerate || briefing.status === 'ready') && (
          <button type="button" className="astr-btn ab-listen" disabled={generating || loading} onClick={listen}>
            <Play size={15} /> {briefing.status === 'ready' ? 'Listen to briefing' : 'Create audio briefing'}
          </button>
        )}
        {audioUrl && <audio ref={audioRef} className="ab-player" controls preload="metadata" src={audioUrl} aria-label="Play your daily briefing" onError={() => { setAudioUrl(null); setError('Playback failed. Please load the audio again.'); }} />}
        {briefing && <details className="ab-transcript"><summary>Read the transcript</summary><p>{briefing.transcript}</p></details>}
        <p className="ab-footnote">AI narration by ElevenLabs. Creating audio shares this briefing text with ElevenLabs. Based on your Western calendar reading.</p>
      </div>
    </section>
  );
}
