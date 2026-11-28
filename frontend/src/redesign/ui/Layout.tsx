import { useId, useMemo, useRef, type HTMLAttributes, type ReactNode } from 'react';
import { cn } from './cn';
import { gsap, reducedMotion, useGSAP, useSettle } from './motion';

/**
 * Page structure. Every screen is a <Page> of <Section>s; each section has one
 * job, a serif title, a one-line description and its content. Sections are
 * separated by space, not lines. Long copy goes behind <ReadMore>.
 */
export function Page({ children, className, wide }: { children: ReactNode; className?: string; wide?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useSettle(ref);
  return (
    <div ref={ref} className={cn('mx-auto flex w-full flex-col gap-14 pb-10 md:gap-20', wide ? 'max-w-[1280px]' : 'max-w-[1120px]', className)}>
      {children}
    </div>
  );
}

export function PageHeader({
  title,
  lead,
  children,
  aside,
  className,
}: {
  title: ReactNode;
  /** One sentence: what this screen is for, in the member's words. */
  lead?: ReactNode;
  /** Controls that belong to the whole page (a picker, a toggle). */
  children?: ReactNode;
  /** Something that sits beside the title on wide screens (a stat, a glyph). */
  aside?: ReactNode;
  className?: string;
}) {
  return (
    <header data-settle className={cn('flex flex-wrap items-end justify-between gap-x-10 gap-y-6', className)}>
      <div className="flex min-w-0 max-w-[760px] flex-1 basis-[420px] flex-col gap-3">
        <h1 className="font-display text-[34px] leading-[1.06] text-ink [text-wrap:balance] md:text-[52px] md:leading-[1.03]">{title}</h1>
        {lead && <p className="max-w-[60ch] text-[17px] leading-relaxed text-ink-2 [text-wrap:pretty] md:text-[19px]">{lead}</p>}
        {children && <div className="mt-2 flex flex-wrap items-center gap-3">{children}</div>}
      </div>
      {aside}
    </header>
  );
}

export function Section({
  title,
  description,
  actions,
  children,
  className,
  id,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  id?: string;
}) {
  const headingId = useId();
  return (
    <section data-settle id={id} aria-labelledby={title ? headingId : undefined} className={cn('flex min-w-0 flex-col gap-6', className)}>
      {(title || description || actions) && (
        <div className="flex flex-wrap items-baseline gap-x-7 gap-y-2">
          {title && (
            <h2 id={headingId} className="font-display text-[28px] leading-[1.1] text-ink [text-wrap:balance] md:text-[38px]">
              {title}
            </h2>
          )}
          {description && <p className="max-w-[58ch] text-[16px] leading-relaxed text-ink-2 md:text-[17px]">{description}</p>}
          {actions && <div className="flex flex-wrap items-center gap-2 md:ml-auto">{actions}</div>}
        </div>
      )}
      {children}
    </section>
  );
}

export function Card({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('min-w-0 rounded-[24px] border border-line bg-vellum p-5 md:p-7', className)} {...rest}>
      {children}
    </div>
  );
}

/** A larger island for a section's main content (the tree, a chart, a story). */
export function Panel({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('min-w-0 rounded-[26px] border border-line bg-vellum p-5 md:rounded-[32px] md:p-10', className)} {...rest}>
      {children}
    </div>
  );
}

/** The night panel: live and "today" content. Gold is its action colour. */
export function NightPanel({ className, children, stars = true, ...rest }: HTMLAttributes<HTMLDivElement> & { stars?: boolean }) {
  return (
    <div className={cn('rd-night relative min-w-0 overflow-hidden rounded-[26px] bg-night p-5 text-night-ink md:rounded-[32px] md:p-11', className)} {...rest}>
      {stars && <Stars />}
      <div className="relative">{children}</div>
    </div>
  );
}

function Stars({ count = 26, seed = 7 }: { count?: number; seed?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const stars = useMemo(() => {
    // A fixed pseudo-random sequence, so the stars sit still between renders.
    const values: number[] = [];
    for (let i = 0, v = seed; i < count * 4; i++) {
      v = (v * 9301 + 49297) % 233280;
      values.push(v / 233280);
    }
    const at = (index: number) => values[index] ?? 0;
    return Array.from({ length: count }, (_, i) => ({
      left: at(i * 4) * 100,
      top: at(i * 4 + 1) * 100,
      size: 1.2 + at(i * 4 + 2) * 1.4,
      opacity: 0.16 + at(i * 4 + 3) * 0.4,
      twinkle: i % 4 === 0,
    }));
  }, [count, seed]);
  useGSAP(
    () => {
      if (reducedMotion()) return;
      gsap.utils.toArray<HTMLElement>('[data-twinkle]').forEach((el, i) => {
        gsap.to(el, { opacity: 0.08, duration: 2.6 + (i % 4) * 0.8, repeat: -1, yoyo: true, ease: 'sine.inOut', delay: i * 0.45 });
      });
    },
    { scope: ref },
  );
  return (
    <div ref={ref} aria-hidden="true" className="pointer-events-none absolute inset-0">
      {stars.map((star, i) => (
        <span
          key={i}
          data-twinkle={star.twinkle ? '' : undefined}
          className="absolute rounded-full bg-night-ink"
          style={{ left: `${star.left}%`, top: `${star.top}%`, width: star.size, height: star.size, opacity: star.opacity }}
        />
      ))}
    </div>
  );
}

/** A small, quiet label for a value inside a card ("Day", "Orb", "Balance"). */
export function Label({ children, className, tone = 'violet' }: { children: ReactNode; className?: string; tone?: 'violet' | 'muted' | 'night' }) {
  return (
    <p className={cn('text-[14px] font-semibold', tone === 'violet' ? 'text-violet-ink' : tone === 'muted' ? 'text-ink-muted' : 'text-night-muted', className)}>
      {children}
    </p>
  );
}
