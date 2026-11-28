import { useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Button } from './Button';
import { cn } from './cn';
import { gsap, reducedMotion } from './motion';

/**
 * A region that opens and closes with a height animation. Closed content gets
 * the `hidden` attribute, so it leaves the accessibility tree too.
 */
export function Collapse({ open, id, children, className }: { open: boolean; id?: string; children: ReactNode; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const first = useRef(true);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (first.current) {
      first.current = false;
      el.hidden = !open;
      return;
    }
    gsap.killTweensOf(el);
    if (reducedMotion()) {
      el.hidden = !open;
      el.style.height = '';
      return;
    }
    let tween: gsap.core.Tween;
    if (open) {
      el.hidden = false;
      tween = gsap.fromTo(el, { height: 0 }, { height: 'auto', duration: 0.42, ease: 'power2.out', clearProps: 'height' });
    } else {
      tween = gsap.to(el, {
        height: 0,
        duration: 0.28,
        ease: 'power2.in',
        onComplete: () => {
          el.hidden = true;
          gsap.set(el, { clearProps: 'height' });
        },
      });
    }
    // A paused tab stops animation frames; finish the move on a timer so the
    // region never stays half open.
    const finish = window.setTimeout(() => {
      if (tween.isActive() || tween.progress() < 1) tween.progress(1);
    }, 700);
    return () => window.clearTimeout(finish);
  }, [open]);

  return (
    <div ref={ref} id={id} className={cn('overflow-hidden', className)}>
      {children}
    </div>
  );
}

/**
 * The redesign's answer to walls of text: show the takeaway, keep the rest one
 * tap away. The button sits under the content and stays in place.
 */
export function ReadMore({
  children,
  label = 'Read more',
  lessLabel = 'Show less',
  tone = 'light',
  className,
  defaultOpen = false,
}: {
  children: ReactNode;
  label?: string;
  lessLabel?: string;
  tone?: 'light' | 'night';
  className?: string;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const id = useId();
  return (
    <div className={cn('flex flex-col', className)}>
      <Collapse open={open} id={id}>
        <div className="pb-2 pt-1">{children}</div>
      </Collapse>
      <Button
        variant={tone === 'night' ? 'textNight' : 'text'}
        chevron
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((value) => !value)}
        className="self-start"
      >
        {open ? lessLabel : label}
      </Button>
    </div>
  );
}

/** A labelled toggle row that opens a region below it ("Sources and method"). */
export function Disclosure({ title, children, className, tone = 'light' }: { title: ReactNode; children: ReactNode; className?: string; tone?: 'light' | 'night' }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <div className={cn('flex flex-col gap-2', className)}>
      <Button
        variant={tone === 'night' ? 'textNight' : 'text'}
        chevron
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((value) => !value)}
        className="self-start"
      >
        {title}
      </Button>
      <Collapse open={open} id={id}>
        {children}
      </Collapse>
    </div>
  );
}
