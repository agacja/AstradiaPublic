import { cn } from './cn';

/**
 * Skeletons replace every spinner and "Loading…" line. They take the shape of
 * the content that is coming, so nothing jumps when it arrives. They pulse
 * gently and stop under reduced motion.
 */
export function Skeleton({ className, tone = 'light' }: { className?: string; tone?: 'light' | 'night' }) {
  return <div aria-hidden="true" className={cn('animate-skeleton rounded-[10px]', tone === 'light' ? 'bg-haze' : 'bg-night-3', className)} />;
}

const LINE_WIDTHS = ['w-full', 'w-[94%]', 'w-[86%]', 'w-[72%]', 'w-[90%]', 'w-[60%]'];

export function SkeletonText({ lines = 3, className, tone = 'light' }: { lines?: number; className?: string; tone?: 'light' | 'night' }) {
  return (
    <div aria-hidden="true" className={cn('flex flex-col gap-2.5', className)}>
      {Array.from({ length: lines }, (_, i) => (
        <Skeleton key={i} tone={tone} className={cn('h-3.5', i === lines - 1 ? 'w-[58%]' : LINE_WIDTHS[i % LINE_WIDTHS.length])} />
      ))}
    </div>
  );
}

export function SkeletonCard({ className, lines = 3 }: { className?: string; lines?: number }) {
  return (
    <div aria-hidden="true" className={cn('flex flex-col gap-4 rounded-[24px] border border-line bg-vellum p-5 md:p-7', className)}>
      <Skeleton className="h-3.5 w-20" />
      <Skeleton className="h-8 w-[55%]" />
      <SkeletonText lines={lines} />
    </div>
  );
}

/** A whole screen on its way: a header and a few sections. Announced once. */
export function ScreenSkeleton({ label = 'Loading', sections = 2 }: { label?: string; sections?: number }) {
  return (
    <div role="status" aria-live="polite" className="mx-auto flex w-full max-w-[1120px] flex-col gap-14 md:gap-20">
      <span className="sr-only">{label}</span>
      <div aria-hidden="true" className="flex flex-col gap-4">
        <Skeleton className="h-10 w-[70%] max-w-[560px] md:h-14" />
        <Skeleton className="h-4 w-[85%] max-w-[620px]" />
        <Skeleton className="h-4 w-[60%] max-w-[420px]" />
      </div>
      {Array.from({ length: sections }, (_, i) => (
        <div key={i} aria-hidden="true" className="flex flex-col gap-6">
          <Skeleton className="h-8 w-[40%] max-w-[320px]" />
          <div className="grid grid-cols-1 gap-5 md:grid-cols-3">
            <SkeletonCard />
            <SkeletonCard className="hidden md:flex" />
            <SkeletonCard className="hidden md:flex" />
          </div>
        </div>
      ))}
    </div>
  );
}
