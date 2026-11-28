import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { cn } from './cn';
import { gsap, reducedMotion } from './motion';

export interface SegmentOption<T extends string> {
  value: T;
  label: ReactNode;
}

/**
 * A segmented control (single choice among 2-5 views). The selected pill
 * slides between options; arrow keys move the choice, as in a radio group.
 */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  label,
  size = 'md',
  tone = 'light',
  className,
}: {
  options: SegmentOption<T>[];
  value: T;
  onChange: (value: T) => void;
  /** Accessible name of the group. */
  label: string;
  size?: 'sm' | 'md';
  tone?: 'light' | 'night';
  className?: string;
}) {
  const wrap = useRef<HTMLDivElement>(null);
  const pill = useRef<HTMLSpanElement>(null);
  const [measured, setMeasured] = useState(false);
  const first = useRef(true);

  useLayoutEffect(() => {
    const host = wrap.current;
    const indicator = pill.current;
    if (!host || !indicator) return;
    const place = (animate: boolean) => {
      const active = host.querySelector<HTMLElement>('[aria-checked="true"]');
      if (!active) return;
      const target = { x: active.offsetLeft, width: active.offsetWidth };
      if (animate && !reducedMotion()) gsap.to(indicator, { ...target, duration: 0.35, ease: 'power3.out' });
      else gsap.set(indicator, target);
      setMeasured(true);
    };
    place(!first.current);
    first.current = false;
    const observer = new ResizeObserver(() => place(false));
    observer.observe(host);
    return () => observer.disconnect();
  }, [value, options.length]);

  const move = (step: number) => {
    const index = options.findIndex((option) => option.value === value);
    const next = options[(index + step + options.length) % options.length];
    if (!next) return;
    onChange(next.value);
    requestAnimationFrame(() => wrap.current?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus());
  };

  const night = tone === 'night';
  return (
    <div
      ref={wrap}
      role="radiogroup"
      aria-label={label}
      className={cn('relative inline-flex max-w-full rounded-[14px] p-1', night ? 'bg-night-2' : 'bg-mist', className)}
      onKeyDown={(event) => {
        if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
          event.preventDefault();
          move(1);
        } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
          event.preventDefault();
          move(-1);
        }
      }}
    >
      <span
        ref={pill}
        aria-hidden="true"
        className={cn('absolute bottom-1 left-0 top-1 rounded-[10px] transition-opacity', night ? 'bg-night-3' : 'bg-vellum shadow-[0_1px_2px_rgb(31_26_61_/_0.08)]', measured ? 'opacity-100' : 'opacity-0')}
      />
      {options.map((option) => {
        const on = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={on}
            tabIndex={on ? 0 : -1}
            onClick={() => onChange(option.value)}
            className={cn(
              'relative z-[1] inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-[10px] font-semibold transition-colors duration-200',
              size === 'md' ? 'min-h-10 px-4 text-[15px]' : 'min-h-8 px-3 text-[13.5px]',
              night ? (on ? 'text-gold-bright' : 'text-night-muted hover:text-night-ink') : on ? 'text-ink' : 'text-ink-muted hover:text-ink',
              !measured && on && (night ? 'bg-night-3' : 'bg-vellum'),
            )}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
