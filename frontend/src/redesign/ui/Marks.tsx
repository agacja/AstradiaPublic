import type { ReactNode } from 'react';
import { cn } from './cn';

/**
 * State is shown in form as well as colour, so it reads without colour vision:
 *   flow / up       a filled gold dot   (supported, flowing, bullish)
 *   test / down     a rose ring         (tested, testing, bearish)
 *   rest / neutral  a thin violet ring  (resting, neutral)
 *   live            a filled violet dot
 */
export type MarkKind = 'flow' | 'up' | 'test' | 'down' | 'rest' | 'neutral' | 'live';

const MARK: Record<MarkKind, { light: string; night: string }> = {
  flow: { light: 'bg-gold-ring', night: 'bg-gold-bright' },
  up: { light: 'bg-gold-ring', night: 'bg-gold-bright' },
  test: { light: 'border-[2.5px] border-rose', night: 'border-[2.5px] border-rose-bright' },
  down: { light: 'border-[2.5px] border-rose', night: 'border-[2.5px] border-rose-bright' },
  rest: { light: 'border-[1.5px] border-[#a79fcb]', night: 'border-[1.5px] border-night-muted' },
  neutral: { light: 'border-[1.5px] border-[#a79fcb]', night: 'border-[1.5px] border-night-muted' },
  live: { light: 'bg-violet', night: 'bg-night-ink' },
};

export function Mark({ kind, tone = 'light', className }: { kind: MarkKind; tone?: 'light' | 'night'; className?: string }) {
  return <span aria-hidden="true" className={cn('inline-block size-[11px] shrink-0 rounded-full', MARK[kind][tone], className)} />;
}

const INK: Record<MarkKind, { light: string; night: string }> = {
  flow: { light: 'text-gold-ink', night: 'text-gold-bright' },
  up: { light: 'text-gold-ink', night: 'text-gold-bright' },
  test: { light: 'text-rose-ink', night: 'text-rose-bright' },
  down: { light: 'text-rose-ink', night: 'text-rose-bright' },
  rest: { light: 'text-ink-muted', night: 'text-night-muted' },
  neutral: { light: 'text-ink-muted', night: 'text-night-muted' },
  live: { light: 'text-violet-ink', night: 'text-night-ink' },
};

/** A mark with its word: "● Supported", "○ Testing". */
export function StateLabel({ kind, children, tone = 'light', className }: { kind: MarkKind; children: ReactNode; tone?: 'light' | 'night'; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2 font-semibold', INK[kind][tone], className)}>
      <Mark kind={kind} tone={tone} />
      {children}
    </span>
  );
}

/** A signed change: "+3.2%" in gold with ▲, "−1.4%" in rose with ▼. */
export function Delta({ value, digits = 2, suffix = '%', tone = 'light', className }: { value: number | null | undefined; digits?: number; suffix?: string; tone?: 'light' | 'night'; className?: string }) {
  if (value === null || value === undefined || Number.isNaN(value)) return <span className={cn('tabular-nums text-ink-muted', className)}>—</span>;
  // Round first, so a tiny change never reads as "▼ −0.00%".
  const shown = Number(value.toFixed(digits));
  const kind: MarkKind = shown > 0 ? 'up' : shown < 0 ? 'down' : 'neutral';
  const sign = shown > 0 ? '+' : shown < 0 ? '−' : '';
  return (
    <span className={cn('inline-flex items-center gap-1 font-semibold tabular-nums', INK[kind][tone], className)}>
      <span aria-hidden="true" className="text-[0.7em]">{shown > 0 ? '▲' : shown < 0 ? '▼' : '•'}</span>
      {sign}
      {Math.abs(shown).toFixed(digits)}
      {suffix}
    </span>
  );
}

/** A horizontal share bar with stable round caps (the fill is a width, never a scale). */
export function Meter({ value, max = 100, tone = 'violet', className, label }: { value: number; max?: number; tone?: 'violet' | 'gold' | 'rose'; className?: string; label?: string }) {
  const pct = Math.max(0, Math.min(100, (value / max) * 100));
  return (
    <span
      role={label ? 'meter' : undefined}
      aria-label={label}
      aria-valuenow={label ? value : undefined}
      aria-valuemin={label ? 0 : undefined}
      aria-valuemax={label ? max : undefined}
      className={cn('block h-2.5 overflow-hidden rounded-full bg-mist', className)}
    >
      <span
        className={cn('block h-full rounded-full', tone === 'violet' ? 'bg-violet' : tone === 'gold' ? 'bg-gold' : 'bg-rose')}
        style={{ width: `${pct}%` }}
      />
    </span>
  );
}

/** Planet and zodiac glyphs, always as text. */
export function Glyph({ children, className, label }: { children: ReactNode; className?: string; label?: string }) {
  return (
    <span className={cn('rd-glyph', className)} role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
      {children}
    </span>
  );
}
