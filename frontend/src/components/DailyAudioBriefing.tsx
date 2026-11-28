import { useEffect, useRef, useState } from 'react';
import { Button, gsap, Icon, NightPanel, ReadMore, reducedMotion, Skeleton } from '../redesign/ui';
import { getAuthToken } from '../utils/session';
import { loadDailyBriefing, loadBriefingAudio, type DailyBriefing } from '../utils/audio-briefing-api';

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
  const actionRef = useRef<AbortController | null>(null);
  const [reload, setReload] = useState(0);
  const [playing, setPlaying] = useState(false);

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
    weekday: 'long', month: 'long', day: 'numeric', timeZone: 'UTC',
  });
  const minutes = briefing ? Math.max(1, Math.round(briefing.estimatedSeconds / 60)) : null;
  const firstSentence = briefing?.transcript.split(/(?<=\.)\s+/)[0] ?? '';
  const rest = briefing ? briefing.transcript.slice(firstSentence.length).trim() : '';

  return (
    <NightPanel aria-label="Your daily briefing">
      <div className="flex flex-col gap-6">
        <div className="flex items-start justify-between gap-6">
          <div className="flex min-w-0 flex-col gap-1.5">
            <h2 className="font-display text-[28px] leading-tight text-night-ink md:text-[36px]">Your daily briefing</h2>
            <p className="text-[15px] text-night-muted">{date ? `${date} · about ${minutes} min` : 'A short daily reading from your natal chart'}</p>
          </div>
          <BriefingWave playing={!!audioUrl && playing} />
        </div>

        <div role="status" aria-live="polite" className="empty:hidden">
          {loading && (
            <div className="flex flex-col gap-3" aria-label="Preparing your reading">
              <Skeleton tone="night" className="h-5 w-[80%]" />
              <Skeleton tone="night" className="h-5 w-[55%]" />
            </div>
          )}
          {generating && <p className="text-[16px] text-night-ink-2">Recording your audio. This takes a moment.</p>}
          {briefing?.status === 'unavailable' && <p className="text-[16px] text-night-ink-2">Audio is not available yet. Your written briefing is below.</p>}
          {briefing?.status === 'limit_reached' && <p className="text-[16px] text-night-ink-2">Today’s audio limit is reached. Your written briefing is still here.</p>}
        </div>

        {briefing && (
          <p className="max-w-[60ch] font-display text-[22px] italic leading-snug text-night-ink md:text-[26px]">{firstSentence}</p>
        )}

        {error && (
          <div role="alert" className="flex flex-wrap items-center gap-3">
            <p className="text-[16px] text-rose-bright">{error}</p>
            <Button variant="textNight" icon="refresh" disabled={busy} onClick={() => setReload(n => n + 1)}>
              Reload the briefing
            </Button>
          </div>
        )}

        {audioUrl ? (
          <AudioPlayer
            src={audioUrl}
            onPlayingChange={setPlaying}
            onError={() => { setAudioUrl(null); setError('Playback failed. Load the audio again.'); }}
          />
        ) : (
          briefing && (briefing.canGenerate || briefing.status === 'ready') && (
            <Button variant="night" size="lg" icon="play" disabled={generating || loading} onClick={listen} className="self-start">
              {generating ? 'Recording…' : briefing.status === 'ready' ? 'Listen' : 'Create the audio'}
            </Button>
          )
        )}

        {briefing && rest && (
          <ReadMore tone="night" label="Read the whole briefing" lessLabel="Show less">
            <p className="max-w-[64ch] text-[16px] leading-relaxed text-night-ink-2">{rest}</p>
          </ReadMore>
        )}
      </div>
    </NightPanel>
  );
}

const BARS = [10, 18, 13, 26, 19, 34, 24, 15, 29, 21, 12, 23, 31, 17, 27, 14, 22, 30, 18, 11];

/** The briefing's mark: rounded bars that move while it plays (heights animate; caps stay round). */
function BriefingWave({ playing }: { playing: boolean }) {
  const ref = useRef<SVGSVGElement>(null);
  useEffect(() => {
    const bars = ref.current?.querySelectorAll('rect');
    if (!bars) return;
    gsap.killTweensOf(bars);
    bars.forEach((bar, i) => {
      const base = BARS[i] ?? 0;
      if (playing && !reducedMotion()) {
        gsap.to(bar, { attr: { height: Math.max(6, base * 0.35), y: 20 - Math.max(6, base * 0.35) / 2 }, duration: 0.35 + (i % 5) * 0.07, repeat: -1, yoyo: true, ease: 'sine.inOut', delay: (i % 7) * 0.05 });
      } else {
        gsap.to(bar, { attr: { height: base, y: 20 - base / 2 }, duration: 0.3, ease: 'power2.out' });
      }
    });
  }, [playing]);
  return (
    <svg ref={ref} viewBox="0 0 120 40" width="150" height="50" aria-hidden="true" className="hidden shrink-0 text-gold-bright sm:block">
      {BARS.map((h, i) => (
        <rect key={i} x={i * 6 + 1} y={20 - h / 2} width={3} height={h} rx={1.5} fill="currentColor" opacity={0.35 + (i % 4) * 0.15} />
      ))}
    </svg>
  );
}

const clock = (seconds: number) => `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, '0')}`;

/** Play/pause, a progress line and the time, in the night panel's gold. */
function AudioPlayer({ src, onError, onPlayingChange }: { src: string; onError: () => void; onPlayingChange: (playing: boolean) => void }) {
  const audio = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);

  useEffect(() => {
    // Some mobile browsers need another tap after asynchronous generation.
    void audio.current?.play().catch(() => {});
  }, [src]);
  useEffect(() => onPlayingChange(playing), [playing, onPlayingChange]);

  const toggle = () => {
    const a = audio.current;
    if (!a) return;
    if (a.paused) void a.play().catch(() => {});
    else a.pause();
  };
  const pct = duration ? Math.min(100, (time / duration) * 100) : 0;

  return (
    <div className="flex items-center gap-4 rounded-[20px] bg-night-2 p-3 pr-5">
      <audio
        ref={audio}
        src={src}
        preload="metadata"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onTimeUpdate={(e) => setTime(e.currentTarget.currentTime)}
        onLoadedMetadata={(e) => setDuration(e.currentTarget.duration || 0)}
        onError={onError}
        aria-label="Your daily briefing"
      />
      <button
        type="button"
        onClick={toggle}
        aria-label={playing ? 'Pause the briefing' : 'Play the briefing'}
        className="inline-flex size-12 shrink-0 items-center justify-center rounded-full bg-gold-bright text-night transition-colors hover:bg-[#f7cf6d]"
      >
        <Icon name={playing ? 'pause' : 'play'} size={22} />
      </button>
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <span className="block h-1.5 overflow-hidden rounded-full bg-night-3">
          <span className="block h-full rounded-full bg-gold-bright" style={{ width: `${pct}%` }} />
        </span>
        <span className="flex justify-between text-[13px] tabular-nums text-night-muted">
          <span>{clock(time)}</span>
          <span>{duration ? clock(duration) : '--:--'}</span>
        </span>
      </div>
    </div>
  );
}
